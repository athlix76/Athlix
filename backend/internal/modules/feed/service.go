package feed

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/media"
	"athlix/backend/internal/platform/userdir"
)

// Service holds the feed business rules.
type Service struct {
	d *app.Deps
	r repo
}

// NewService builds the feed service. No I/O.
func NewService(d *app.Deps) *Service { return &Service{d: d} }

var (
	errPostNotFound    = httpx.NotFound("Post")
	errCommentNotFound = httpx.NotFound("Comment")
	errUserNotFound    = httpx.NotFound("User")
)

// PageSize is feed.page_size capped at the platform maximum of 100.
func (s *Service) PageSize(ctx context.Context) int {
	n := s.d.Settings.Int(ctx, "feed.page_size", 20)
	if n <= 0 {
		n = 20
	}
	if n > 100 {
		n = 100
	}
	return n
}

// Feed returns one page of the viewer's feed in mode following or discover.
func (s *Service) Feed(ctx context.Context, viewer int64, mode string, p httpx.PageParams) ([]Post, string, error) {
	switch mode {
	case ModeFollowing:
		rows, err := s.r.listFollowing(ctx, s.d.DB, viewer, p.Cursor.ID, p.Limit+1)
		if err != nil {
			return nil, "", err
		}
		rows, next := trimPage(rows, p.Limit, "")
		posts, err := s.build(ctx, viewer, rows, nil)
		return posts, next, err
	case ModeDiscover:
		return s.discover(ctx, viewer, p)
	default:
		return nil, "", httpx.Validation(map[string]string{"mode": "must be following or discover"})
	}
}

// discover pages public posts in two phases: first the viewer's sports ("s"), then every
// other public post ("o"). Each phase pages by descending id; the phase rides in Cursor.Time.
func (s *Service) discover(ctx context.Context, viewer int64, p httpx.PageParams) ([]Post, string, error) {
	phase := p.Cursor.Time
	if phase != "" && phase != phaseMySports && phase != phaseOther {
		return nil, "", httpx.Validation(map[string]string{"cursor": "is invalid"})
	}
	sports, err := s.r.mySports(ctx, s.d.DB, viewer)
	if err != nil {
		return nil, "", err
	}
	var rows []postRow
	next := ""
	if len(sports) == 0 || phase == phaseOther {
		before := int64(0)
		if phase == phaseOther {
			before = p.Cursor.ID
		}
		rows, err = s.r.listDiscover(ctx, s.d.DB, viewer, sports, false, before, p.Limit+1)
		if err != nil {
			return nil, "", err
		}
		rows, next = trimPage(rows, p.Limit, phaseOther)
	} else {
		rows, err = s.r.listDiscover(ctx, s.d.DB, viewer, sports, true, p.Cursor.ID, p.Limit+1)
		if err != nil {
			return nil, "", err
		}
		if len(rows) > p.Limit {
			rows, next = trimPage(rows, p.Limit, phaseMySports)
		} else if remaining := p.Limit - len(rows); remaining == 0 {
			next = httpx.EncodeCursor(httpx.Cursor{Time: phaseOther})
		} else {
			more, err := s.r.listDiscover(ctx, s.d.DB, viewer, sports, false, 0, remaining+1)
			if err != nil {
				return nil, "", err
			}
			more, next = trimPage(more, remaining, phaseOther)
			rows = append(rows, more...)
		}
	}
	posts, err := s.build(ctx, viewer, rows, nil)
	return posts, next, err
}

// trimPage cuts rows (fetched with limit+1) to limit and returns the next cursor, or "" on
// the last page.
func trimPage(rows []postRow, limit int, phase string) ([]postRow, string) {
	if len(rows) <= limit {
		return rows, ""
	}
	rows = rows[:limit]
	return rows, httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID, Time: phase})
}

// UserPosts returns the posts of userID that viewer may see. 404 when the user is not
// active or a block exists either way.
func (s *Service) UserPosts(ctx context.Context, viewer, userID int64, p httpx.PageParams) ([]Post, string, error) {
	if userID != viewer {
		ok, err := s.r.activeUser(ctx, s.d.DB, userID)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			return nil, "", errUserNotFound
		}
		blocked, err := s.d.Users.BlockedEitherWay(ctx, viewer, userID)
		if err != nil {
			return nil, "", err
		}
		if blocked {
			return nil, "", errUserNotFound
		}
	}
	rows, err := s.r.listByAuthor(ctx, s.d.DB, viewer, userID, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", err
	}
	rows, next := trimPage(rows, p.Limit, "")
	posts, err := s.build(ctx, viewer, rows, nil)
	return posts, next, err
}

// GetPost returns one post if viewer may see it, else 404.
func (s *Service) GetPost(ctx context.Context, viewer, id int64) (Post, error) {
	row, err := s.r.visiblePost(ctx, s.d.DB, viewer, id)
	if db.IsNoRows(err) {
		return Post{}, errPostNotFound
	}
	if err != nil {
		return Post{}, err
	}
	posts, err := s.build(ctx, viewer, []postRow{row}, nil)
	if err != nil {
		return Post{}, err
	}
	if len(posts) == 0 {
		return Post{}, errPostNotFound
	}
	return posts[0], nil
}

// validatePost checks the request shape and returns the cleaned body and visibility.
func (s *Service) validatePost(ctx context.Context, u userdir.Basic, in CreatePostInput) (string, string, map[string]string) {
	fields := map[string]string{}
	body := ""
	if in.Body != nil {
		body = strings.TrimSpace(*in.Body)
	}
	if maxLen := min(s.d.Settings.Int(ctx, "feed.post_max_length", 2000), postBodyColumnMax); utf8.RuneCountInString(body) > maxLen {
		fields["body"] = "is too long"
	}
	if maxMedia := s.d.Settings.Int(ctx, "feed.post_max_media", 4); len(in.MediaUploadIDs) > maxMedia {
		fields["mediaUploadIds"] = "has too many images"
	} else {
		seen := map[int64]bool{}
		for _, id := range in.MediaUploadIDs {
			if id <= 0 || seen[id] {
				fields["mediaUploadIds"] = "contains an invalid or repeated id"
				break
			}
			seen[id] = true
		}
	}
	if body == "" && len(in.MediaUploadIDs) == 0 {
		fields["body"] = "add text or at least one image"
	}
	if in.SportID != nil && *in.SportID <= 0 {
		fields["sportId"] = "is invalid"
	}
	if in.EventID != nil && *in.EventID <= 0 {
		fields["eventId"] = "is invalid"
	}
	minorRestricted := u.IsMinor && s.d.Settings.String(ctx, "minor.profile_default_visibility", "followers") != "public"
	visibility := "public"
	if minorRestricted {
		visibility = "followers"
	}
	if in.Visibility != nil {
		switch *in.Visibility {
		case "followers":
			visibility = "followers"
		case "public":
			if minorRestricted {
				fields["visibility"] = "public posts are not available for this account"
			}
		default:
			fields["visibility"] = "must be public or followers"
		}
	}
	return body, visibility, fields
}

// CreatePost validates, claims the images and inserts the post. u must be the token's user.
func (s *Service) CreatePost(ctx context.Context, u userdir.Basic, in CreatePostInput) (Post, error) {
	body, visibility, fields := s.validatePost(ctx, u, in)
	if len(fields) > 0 {
		return Post{}, httpx.Validation(fields)
	}
	var sportID, eventID int64
	if in.SportID != nil {
		sportID = *in.SportID
		ok, err := s.r.sportActive(ctx, s.d.DB, sportID)
		if err != nil {
			return Post{}, err
		}
		if !ok {
			fields["sportId"] = "is not an active sport"
		}
	}
	if in.EventID != nil {
		eventID = *in.EventID
		ok, err := s.r.eventLinkable(ctx, s.d.DB, eventID)
		if err != nil {
			return Post{}, err
		}
		if !ok {
			fields["eventId"] = "is not a published event"
		}
	}
	if len(fields) > 0 {
		return Post{}, httpx.Validation(fields)
	}
	mediaIDs := in.MediaUploadIDs
	if mediaIDs == nil {
		mediaIDs = []int64{}
	}
	mediaJSON, _ := json.Marshal(mediaIDs)
	var id int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		for _, mid := range mediaIDs {
			if err := s.d.Media.Claim(ctx, tx, u.ID, mid, media.PurposePostImage); err != nil {
				if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrNotUploaded) || errors.Is(err, media.ErrMismatch) {
					return httpx.Validation(map[string]string{"mediaUploadIds": "contains an image that is not available"})
				}
				return err
			}
		}
		var err error
		id, err = s.r.insertPost(ctx, tx, u.ID, body, mediaJSON, sportID, eventID, visibility)
		return err
	})
	if err != nil {
		return Post{}, err
	}
	logger.From(ctx).Info("post created", slog.Int64("postId", id), slog.Int64("userId", u.ID))
	row := postRow{ID: id, AuthorID: u.ID, MediaIDs: mediaIDs, Visibility: visibility, CreatedAt: s.d.Now().UTC()}
	if body != "" {
		row.Body = sql.NullString{String: body, Valid: true}
	}
	if sportID > 0 {
		row.SportID = sql.NullInt64{Int64: sportID, Valid: true}
	}
	if eventID > 0 {
		row.EventID = sql.NullInt64{Int64: eventID, Valid: true}
	}
	posts, err := s.build(ctx, u.ID, []postRow{row}, map[int64]bool{})
	if err != nil {
		return Post{}, err
	}
	if len(posts) == 0 {
		return Post{}, errors.New("feed: author summary missing after create")
	}
	return posts[0], nil
}

// DeletePost soft-deletes the caller's own post; anyone else's post is 404.
func (s *Service) DeletePost(ctx context.Context, viewer, id int64) error {
	ok, err := s.r.deletePost(ctx, s.d.DB, id, viewer)
	if err != nil {
		return err
	}
	if !ok {
		return errPostNotFound
	}
	return nil
}

// Like adds the viewer's like (idempotent). post.liked is published only for a new like by
// someone other than the author.
func (s *Service) Like(ctx context.Context, viewer, postID int64) (LikeResult, error) {
	var post postRow
	var added bool
	var count int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		if post, err = s.r.visiblePost(ctx, tx, viewer, postID); err != nil {
			if db.IsNoRows(err) {
				return errPostNotFound
			}
			return err
		}
		if added, err = s.r.addLike(ctx, tx, postID, viewer); err != nil {
			return err
		}
		count, err = s.r.likeCount(ctx, tx, postID)
		return err
	})
	if err != nil {
		return LikeResult{}, err
	}
	if added && post.AuthorID != viewer {
		s.publish(ctx, events.PostLiked, viewer, events.PostPayload{PostID: postID, AuthorID: post.AuthorID, ActorID: viewer})
	}
	return LikeResult{Liked: true, LikeCount: count}, nil
}

// Unlike removes the viewer's like (idempotent).
func (s *Service) Unlike(ctx context.Context, viewer, postID int64) (LikeResult, error) {
	var count int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.r.visiblePost(ctx, tx, viewer, postID); err != nil {
			if db.IsNoRows(err) {
				return errPostNotFound
			}
			return err
		}
		if _, err := s.r.removeLike(ctx, tx, postID, viewer); err != nil {
			return err
		}
		var err error
		count, err = s.r.likeCount(ctx, tx, postID)
		return err
	})
	if err != nil {
		return LikeResult{}, err
	}
	return LikeResult{Liked: false, LikeCount: count}, nil
}

// Comments lists a visible post's comments, oldest first.
func (s *Service) Comments(ctx context.Context, viewer, postID int64, p httpx.PageParams) ([]Comment, string, error) {
	if _, err := s.r.visiblePost(ctx, s.d.DB, viewer, postID); err != nil {
		if db.IsNoRows(err) {
			return nil, "", errPostNotFound
		}
		return nil, "", err
	}
	rows, err := s.r.listComments(ctx, s.d.DB, viewer, postID, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	out := []Comment{}
	if len(rows) == 0 {
		return out, next, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.AuthorID)
	}
	authors, err := s.d.Users.Summaries(ctx, uniq(ids))
	if err != nil {
		return nil, "", err
	}
	for _, r := range rows {
		a, ok := authors[r.AuthorID]
		if !ok {
			continue
		}
		out = append(out, Comment{ID: r.ID, PostID: r.PostID, Author: a, Body: r.Body, IsOwn: r.AuthorID == viewer, CreatedAt: r.CreatedAt})
	}
	return out, next, nil
}

// AddComment adds a comment to a visible post. post.commented is published when the
// commenter is not the post author.
func (s *Service) AddComment(ctx context.Context, viewer, postID int64, in CommentInput) (Comment, error) {
	body := strings.TrimSpace(in.Body)
	maxLen := max(1, min(s.d.Settings.Int(ctx, "feed.comment_max_length", commentBodyColumnMax), commentBodyColumnMax))
	if n := utf8.RuneCountInString(body); n == 0 || n > maxLen {
		return Comment{}, httpx.Validation(map[string]string{"body": "must be 1 to " + strconv.Itoa(maxLen) + " characters"})
	}
	var post postRow
	var id int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		if post, err = s.r.visiblePost(ctx, tx, viewer, postID); err != nil {
			if db.IsNoRows(err) {
				return errPostNotFound
			}
			return err
		}
		id, err = s.r.insertComment(ctx, tx, postID, viewer, body)
		return err
	})
	if err != nil {
		return Comment{}, err
	}
	author, ok, err := s.d.Users.Summary(ctx, viewer)
	if post.AuthorID != viewer {
		s.publish(ctx, events.PostCommented, viewer, events.PostPayload{PostID: postID, AuthorID: post.AuthorID, ActorID: viewer, CommentID: id})
	}
	if err != nil {
		return Comment{}, err
	}
	if !ok {
		return Comment{}, errors.New("feed: commenter summary missing")
	}
	return Comment{ID: id, PostID: postID, Author: author, Body: body, IsOwn: true, CreatedAt: s.d.Now().UTC()}, nil
}

// DeleteComment soft-deletes a comment the caller wrote or that sits on the caller's post.
func (s *Service) DeleteComment(ctx context.Context, viewer, commentID int64) error {
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		postID, err := s.r.deletableComment(ctx, tx, commentID, viewer)
		if db.IsNoRows(err) {
			return errCommentNotFound
		}
		if err != nil {
			return err
		}
		return s.r.deleteComment(ctx, tx, commentID, postID)
	})
}

// build turns visible rows into API posts: authors, sports, signed images and likedByMe are
// each loaded in one batch. liked may be passed when already known (e.g. a new post). Rows
// whose author has no summary are dropped. Images are signed only here, i.e. after the
// visibility check done by the query that produced rows.
func (s *Service) build(ctx context.Context, viewer int64, rows []postRow, liked map[int64]bool) ([]Post, error) {
	out := []Post{}
	if len(rows) == 0 {
		return out, nil
	}
	var authorIDs, sportIDs, mediaIDs, postIDs []int64
	for _, r := range rows {
		authorIDs = append(authorIDs, r.AuthorID)
		postIDs = append(postIDs, r.ID)
		if r.SportID.Valid {
			sportIDs = append(sportIDs, r.SportID.Int64)
		}
		mediaIDs = append(mediaIDs, r.MediaIDs...)
	}
	authors, err := s.d.Users.Summaries(ctx, uniq(authorIDs))
	if err != nil {
		return nil, err
	}
	sports := map[int64]SportRef{}
	if len(sportIDs) > 0 {
		if sports, err = s.r.sports(ctx, s.d.DB, uniq(sportIDs)); err != nil {
			return nil, err
		}
	}
	urls := map[int64]string{}
	if len(mediaIDs) > 0 {
		urls = s.d.Media.ViewURLs(ctx, uniq(mediaIDs))
	}
	if liked == nil {
		if liked, err = s.r.likedBy(ctx, s.d.DB, viewer, postIDs); err != nil {
			return nil, err
		}
	}
	for _, r := range rows {
		a, ok := authors[r.AuthorID]
		if !ok {
			continue
		}
		p := Post{
			ID: r.ID, Author: a, Images: []string{}, Visibility: r.Visibility,
			LikeCount: r.LikeCount, CommentCount: r.CommentCount, LikedByMe: liked[r.ID],
			IsOwn: r.AuthorID == viewer, CreatedAt: r.CreatedAt,
		}
		if r.Body.Valid {
			b := r.Body.String
			p.Body = &b
		}
		for _, mid := range r.MediaIDs {
			if u, ok := urls[mid]; ok && u != "" {
				p.Images = append(p.Images, u)
			}
		}
		if r.SportID.Valid {
			if sp, ok := sports[r.SportID.Int64]; ok {
				p.Sport = &sp
			}
		}
		if r.EventID.Valid {
			e := r.EventID.Int64
			p.EventID = &e
		}
		out = append(out, p)
	}
	return out, nil
}

// publish sends an event after commit. A failure is logged (ids only) and does not fail the
// request, because the write has already committed.
func (s *Service) publish(ctx context.Context, typ string, actor int64, payload events.PostPayload) {
	if err := s.d.Events.Publish(ctx, events.New(typ, actor, payload)); err != nil {
		logger.From(ctx).Error("feed event publish failed", slog.String("type", typ), slog.Int64("postId", payload.PostID), slog.String("err", err.Error()))
	}
}

func uniq(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
