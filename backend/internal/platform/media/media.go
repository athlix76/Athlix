// Package media owns the media_uploads table and EVERY object key in the media bucket.
//
// Flow (docs/flows/media-upload.md):
//  1. App: POST /v1/media/uploads {purpose, contentType, sizeBytes} -> CreateUpload builds the key
//     "<purpose>/<userId-from-token>/<uuid>.<ext>", returns {uploadId, uploadUrl, headers}.
//  2. App PUTs the bytes to uploadUrl (presigned, content type + length locked, 5 min).
//  3. App: POST /v1/media/uploads/:id/confirm -> Confirm Heads the object and checks size/type.
//  4. A module that stores an upload id (profile photo, event banner, ...) calls Claim, which
//     checks owner + purpose + status and marks it attached.
//
// Clients NEVER send keys or paths; they only ever see upload ids and signed view URLs.
package media

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/settings"
	"athlix/backend/internal/platform/storage"
)

// Purposes (must match the media_uploads.purpose ENUM).
const (
	PurposeProfilePhoto      = "profile_photo"
	PurposeAchievementCert   = "achievement_certificate"
	PurposeEventBanner       = "event_banner"
	PurposeChatImage         = "chat_image"
	PurposePostImage         = "post_image"
	PurposeCommunityAvatar   = "community_avatar"
	PurposeOrganiserDocument = "organiser_document"
	PurposeLocationProof     = "location_proof" // location identity proof: reviewer-only, deleted after review
)

var documentPurposes = []string{PurposeAchievementCert, PurposeOrganiserDocument, PurposeLocationProof}

// restrictedPurposes are never signed by ViewURL(s); only ReviewerViewURL signs them.
var restrictedPurposes = []string{PurposeLocationProof}

// AllPurposes is the closed list accepted by CreateUpload.
var AllPurposes = []string{PurposeProfilePhoto, PurposeAchievementCert, PurposeEventBanner, PurposeChatImage, PurposePostImage, PurposeCommunityAvatar, PurposeOrganiserDocument, PurposeLocationProof}

var extByType = map[string]string{
	"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "application/pdf": "pdf",
}

// Errors returned to callers (map them to httpx errors in handlers).
var (
	ErrBadPurpose     = errors.New("media: unknown purpose")
	ErrBadContentType = errors.New("media: content type not allowed")
	ErrTooLarge       = errors.New("media: file too large")
	ErrNotFound       = errors.New("media: upload not found")
	ErrNotUploaded    = errors.New("media: file not uploaded yet")
	ErrMismatch       = errors.New("media: uploaded file does not match")
)

// Upload is a media_uploads row.
type Upload struct {
	ID          int64
	UserID      int64
	Purpose     string
	Key         string
	ContentType string
	SizeBytes   int64
	Status      string
}

// Ticket is returned to the client for step 2.
type Ticket struct {
	UploadID  int64             `json:"uploadId"`
	UploadURL string            `json:"uploadUrl"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

// Service implements the flow above.
type Service struct {
	db       *sql.DB
	store    storage.Storage
	settings *settings.Store
}

// New builds the media service.
func New(d *sql.DB, s storage.Storage, st *settings.Store) *Service {
	return &Service{db: d, store: s, settings: st}
}

func (s *Service) limits(ctx context.Context, purpose string) (types []string, maxBytes int64) {
	if slices.Contains(documentPurposes, purpose) {
		return s.settings.StringList(ctx, "media.allowed_document_types", []string{"application/pdf", "image/jpeg", "image/png"}),
			int64(s.settings.Int(ctx, "media.document_max_mb", 10)) << 20
	}
	return s.settings.StringList(ctx, "media.allowed_image_types", []string{"image/jpeg", "image/png", "image/webp"}),
		int64(s.settings.Int(ctx, "media.image_max_mb", 5)) << 20
}

// CreateUpload validates purpose/type/size, builds the key and returns a presigned PUT.
// userID MUST come from the verified token.
func (s *Service) CreateUpload(ctx context.Context, userID int64, purpose, contentType string, size int64) (Ticket, error) {
	if !slices.Contains(AllPurposes, purpose) {
		return Ticket{}, ErrBadPurpose
	}
	types, maxBytes := s.limits(ctx, purpose)
	ext, known := extByType[contentType]
	if !known || !slices.Contains(types, contentType) {
		return Ticket{}, ErrBadContentType
	}
	if size <= 0 || size > maxBytes {
		return Ticket{}, ErrTooLarge
	}
	key := purpose + "/" + strconv.FormatInt(userID, 10) + "/" + uuid.NewString() + "." + ext
	res, err := s.db.ExecContext(ctx, `INSERT INTO media_uploads (user_id, purpose, s3_key, content_type, size_bytes, status) VALUES (?, ?, ?, ?, ?, 'pending')`,
		userID, purpose, key, contentType, size)
	if err != nil {
		return Ticket{}, err
	}
	id, _ := res.LastInsertId()
	ttl := s.settings.Seconds(ctx, "media.upload_url_ttl_seconds", 300)
	if ttl > 5*time.Minute {
		ttl = 5 * time.Minute
	}
	url, headers, err := s.store.PresignPut(ctx, key, contentType, size, ttl)
	if err != nil {
		return Ticket{}, err
	}
	return Ticket{UploadID: id, UploadURL: url, Method: "PUT", Headers: headers, ExpiresAt: time.Now().Add(ttl).UTC()}, nil
}

func (s *Service) get(ctx context.Context, q db.Querier, userID, uploadID int64, forUpdate bool) (Upload, error) {
	sqlq := `SELECT id, user_id, purpose, s3_key, content_type, size_bytes, status FROM media_uploads WHERE id = ? AND user_id = ? AND status <> 'deleted'`
	if forUpdate {
		sqlq += " FOR UPDATE"
	}
	var u Upload
	err := q.QueryRowContext(ctx, sqlq, uploadID, userID).Scan(&u.ID, &u.UserID, &u.Purpose, &u.Key, &u.ContentType, &u.SizeBytes, &u.Status)
	if db.IsNoRows(err) {
		return Upload{}, ErrNotFound
	}
	return u, err
}

// Confirm checks the object exists with the declared size and content type.
func (s *Service) Confirm(ctx context.Context, userID, uploadID int64) (Upload, error) {
	u, err := s.get(ctx, s.db, userID, uploadID, false)
	if err != nil {
		return Upload{}, err
	}
	if u.Status != "pending" {
		return u, nil
	}
	if err := s.verifyObject(ctx, u); err != nil {
		return Upload{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE media_uploads SET status = 'confirmed', confirmed_at = CURRENT_TIMESTAMP(3) WHERE id = ? AND user_id = ? AND status = 'pending'`, u.ID, userID)
	u.Status = "confirmed"
	return u, err
}

func (s *Service) verifyObject(ctx context.Context, u Upload) error {
	obj, err := s.store.Head(ctx, u.Key)
	if errors.Is(err, storage.ErrNotFound) {
		return ErrNotUploaded
	}
	if err != nil {
		return err
	}
	if obj.Size != u.SizeBytes || !strings.EqualFold(strings.TrimSpace(strings.Split(obj.ContentType, ";")[0]), u.ContentType) {
		_ = s.store.Delete(ctx, u.Key)
		_, _ = s.db.ExecContext(ctx, `UPDATE media_uploads SET status = 'deleted' WHERE id = ?`, u.ID)
		logger.From(ctx).Warn("media upload mismatch", slog.Int64("uploadId", u.ID))
		return ErrMismatch
	}
	return nil
}

// Claim attaches uploadID to a record. It returns ErrNotFound unless the upload belongs to
// userID and has the expected purpose. Pending uploads are verified first. Pass a tx (q) when
// the claim is part of a bigger write. Claiming an already attached upload of the same owner
// and purpose is allowed (re-saving a form).
func (s *Service) Claim(ctx context.Context, q db.Querier, userID, uploadID int64, purpose string) error {
	if q == nil {
		q = s.db
	}
	u, err := s.get(ctx, q, userID, uploadID, false)
	if err != nil {
		return err
	}
	if u.Purpose != purpose {
		return ErrNotFound
	}
	if u.Status == "pending" {
		if err := s.verifyObject(ctx, u); err != nil {
			return err
		}
	}
	_, err = q.ExecContext(ctx, `UPDATE media_uploads SET status = 'attached', confirmed_at = COALESCE(confirmed_at, CURRENT_TIMESTAMP(3)) WHERE id = ? AND user_id = ?`, u.ID, userID)
	return err
}

// ViewURL signs one upload for viewing. The CALLER must already have checked that the viewer
// may see the record the upload is attached to. Returns "" if missing.
func (s *Service) ViewURL(ctx context.Context, uploadID int64) string {
	m := s.ViewURLs(ctx, []int64{uploadID})
	return m[uploadID]
}

// ViewURLs signs many uploads (missing ids are omitted). Same caller rule as ViewURL.
func (s *Service) ViewURLs(ctx context.Context, ids []int64) map[int64]string {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, s3_key FROM media_uploads WHERE status IN ('confirmed','attached') AND purpose <> 'location_proof' AND id IN (`+db.Placeholders(len(ids))+`)`, db.Int64Args(ids)...)
	if err != nil {
		logger.From(ctx).Error("media view urls", slog.String("err", err.Error()))
		return out
	}
	defer rows.Close()
	ttl := s.settings.Seconds(ctx, "media.view_url_ttl_seconds", 900)
	for rows.Next() {
		var id int64
		var key string
		if rows.Scan(&id, &key) != nil {
			continue
		}
		if u, err := s.store.PresignGet(ctx, key, ttl); err == nil {
			out[id] = u
		}
	}
	return out
}

// OwnerOf returns the owner of an upload (for moderation tooling).
func (s *Service) OwnerOf(ctx context.Context, uploadID int64) (int64, error) {
	var uid int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM media_uploads WHERE id = ?`, uploadID).Scan(&uid)
	if db.IsNoRows(err) {
		return 0, ErrNotFound
	}
	return uid, err
}

// ReviewerViewURL signs a restricted upload (location_proof) for an admin reviewer with a short
// TTL (location.proof_view_url_ttl_seconds, max 5 min). The CALLER must hold the permission and
// write the audit row. Returns "" when missing or purged.
func (s *Service) ReviewerViewURL(ctx context.Context, uploadID int64) string {
	var key, purpose string
	err := s.db.QueryRowContext(ctx, `SELECT s3_key, purpose FROM media_uploads WHERE id = ? AND status IN ('confirmed','attached')`, uploadID).Scan(&key, &purpose)
	if err != nil || !slices.Contains(restrictedPurposes, purpose) {
		return ""
	}
	ttl := s.settings.Seconds(ctx, "location.proof_view_url_ttl_seconds", 120)
	if ttl > 5*time.Minute {
		ttl = 5 * time.Minute
	}
	u, err := s.store.PresignGet(ctx, key, ttl)
	if err != nil {
		return ""
	}
	return u
}

// Purge deletes the stored object and marks the upload deleted. Idempotent: purging a deleted
// or missing upload is a no-op. Used for location proofs after review and for user deletion.
func (s *Service) Purge(ctx context.Context, uploadID int64) error {
	var key, status string
	err := s.db.QueryRowContext(ctx, `SELECT s3_key, status FROM media_uploads WHERE id = ?`, uploadID).Scan(&key, &status)
	if db.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if status == "deleted" {
		return nil
	}
	if err := s.store.Delete(ctx, key); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE media_uploads SET status = 'deleted' WHERE id = ?`, uploadID)
	return err
}
