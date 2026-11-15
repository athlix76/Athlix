package media

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	pmedia "athlix/backend/internal/platform/media"
)

// AdminOwnerID owns uploads made from the admin panel (ad images).
const AdminOwnerID int64 = 0

// cleanupBatch is the job batch size (docs/architecture.md: at most 500 rows per run).
const cleanupBatch = 500

// Service wraps platform/media with the HTTP error mapping and the cleanup job.
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

// mapErr turns platform/media errors into API errors.
func mapErr(err error) error {
	switch {
	case errors.Is(err, pmedia.ErrBadPurpose):
		return httpx.Validation(map[string]string{"purpose": "is not a known upload purpose"})
	case errors.Is(err, pmedia.ErrBadContentType):
		return httpx.Validation(map[string]string{"contentType": "is not allowed for this purpose"})
	case errors.Is(err, pmedia.ErrTooLarge):
		return httpx.Validation(map[string]string{"sizeBytes": "must be more than 0 and within the size limit"})
	case errors.Is(err, pmedia.ErrNotFound):
		return httpx.NotFound("Upload")
	case errors.Is(err, pmedia.ErrNotUploaded):
		return httpx.InvalidState("The file has not been uploaded yet.")
	case errors.Is(err, pmedia.ErrMismatch):
		return httpx.Validation(map[string]string{"file": "does not match the size or type you declared; start a new upload"})
	}
	return err
}

// Create is POST /media/uploads: the backend builds the object key from the purpose and
// the caller id; the client only receives a short-lived PUT URL.
func (s *Service) Create(ctx context.Context, userID int64, in uploadBody) (pmedia.Ticket, error) {
	t, err := s.d.Media.CreateUpload(ctx, userID, in.Purpose, in.ContentType, in.SizeBytes)
	if err != nil {
		return pmedia.Ticket{}, mapErr(err)
	}
	return t, nil
}

// CreateAdmin is POST /admin/media/uploads: purpose post_image only, owned by user id 0.
func (s *Service) CreateAdmin(ctx context.Context, in uploadBody) (pmedia.Ticket, error) {
	if in.Purpose != pmedia.PurposePostImage {
		return pmedia.Ticket{}, httpx.Validation(map[string]string{"purpose": "must be post_image"})
	}
	return s.Create(ctx, AdminOwnerID, in)
}

// Confirm is POST /media/uploads/:id/confirm (scoped to the caller: another user's id → 404).
func (s *Service) Confirm(ctx context.Context, userID, uploadID int64) (Confirmed, error) {
	u, err := s.d.Media.Confirm(ctx, userID, uploadID)
	if err != nil {
		return Confirmed{}, mapErr(err)
	}
	// attached uploads were confirmed earlier; the contract only knows "confirmed".
	return Confirmed{UploadID: u.ID, Status: "confirmed"}, nil
}

// ConfirmAdmin is POST /admin/media/uploads/:id/confirm: only uploads owned by user id 0
// (admin uploads) can be confirmed here; any other id → 404.
func (s *Service) ConfirmAdmin(ctx context.Context, uploadID int64) (Confirmed, error) {
	return s.Confirm(ctx, AdminOwnerID, uploadID)
}

// CleanupPending is the media.cleanup_pending job: unconfirmed uploads older than
// media.pending_cleanup_hours lose their object and their row.
func (s *Service) CleanupPending(ctx context.Context) error {
	hours := s.d.Settings.Int(ctx, "media.pending_cleanup_hours", 24)
	if hours < 1 {
		hours = 1
	}
	cutoff := s.d.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	rows, err := s.repo.stalePending(ctx, cutoff, 0, cleanupBatch)
	if err != nil {
		return err
	}
	removed := 0
	for _, p := range rows {
		log := logger.From(ctx).With(slog.Int64("uploadId", p.ID))
		// Object first: if the row delete fails, the next run retries both (Delete is a no-op
		// for a missing object). The other order could leave an orphan object.
		if err := s.d.Storage.Delete(ctx, p.Key); err != nil {
			log.Warn("media cleanup: object delete failed")
			continue
		}
		ok, err := s.repo.deletePending(ctx, p.ID)
		if err != nil {
			return err
		}
		if ok {
			removed++
		}
	}
	logger.From(ctx).Info("media.cleanup_pending done", slog.Int("removed", removed), slog.Int("batch", len(rows)))
	return nil
}
