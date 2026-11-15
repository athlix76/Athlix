package feed

import (
	"database/sql"
	"time"

	"athlix/backend/internal/platform/userdir"
)

// commentBodyColumnMax is the size of post_comments.body (VARCHAR(1000));
// feed.comment_max_length is clamped to it. It is also the setting's default (seed in 0011).
const commentBodyColumnMax = 1000

// postBodyColumnMax is the size of posts.body; feed.post_max_length is clamped to it so a
// larger setting can never overflow the column.
const postBodyColumnMax = 2000

// Feed modes accepted by GET /feed.
const (
	ModeFollowing = "following"
	ModeDiscover  = "discover"
)

// Discover cursor phases: "s" = posts in the viewer's sports, "o" = all other public posts.
const (
	phaseMySports = "s"
	phaseOther    = "o"
)

// SportRef is {id, code, name} (docs/api.md "SportRef").
type SportRef struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Post is the API shape (docs/api.md "Post").
type Post struct {
	ID           int64           `json:"id"`
	Author       userdir.Summary `json:"author"`
	Body         *string         `json:"body"`
	Images       []string        `json:"images"`
	Sport        *SportRef       `json:"sport"`
	EventID      *int64          `json:"eventId"`
	Visibility   string          `json:"visibility"`
	LikeCount    int64           `json:"likeCount"`
	CommentCount int64           `json:"commentCount"`
	LikedByMe    bool            `json:"likedByMe"`
	IsOwn        bool            `json:"isOwn"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// Comment is the API shape (docs/api.md "Comment").
type Comment struct {
	ID        int64           `json:"id"`
	PostID    int64           `json:"postId"`
	Author    userdir.Summary `json:"author"`
	Body      string          `json:"body"`
	IsOwn     bool            `json:"isOwn"`
	CreatedAt time.Time       `json:"createdAt"`
}

// LikeResult is returned by POST/DELETE /posts/:id/like.
type LikeResult struct {
	Liked     bool  `json:"liked"`
	LikeCount int64 `json:"likeCount"`
}

// CreatePostInput is the POST /posts body.
type CreatePostInput struct {
	Body           *string `json:"body"`
	MediaUploadIDs []int64 `json:"mediaUploadIds"`
	SportID        *int64  `json:"sportId"`
	EventID        *int64  `json:"eventId"`
	Visibility     *string `json:"visibility"`
}

// CommentInput is the POST /posts/:id/comments body.
type CommentInput struct {
	Body string `json:"body"`
}

// postRow is a posts row as read by the repository.
type postRow struct {
	ID           int64
	AuthorID     int64
	Body         sql.NullString
	MediaIDs     []int64
	SportID      sql.NullInt64
	EventID      sql.NullInt64
	Visibility   string
	LikeCount    int64
	CommentCount int64
	CreatedAt    time.Time
}

// commentRow is a post_comments row.
type commentRow struct {
	ID        int64
	PostID    int64
	AuthorID  int64
	Body      string
	CreatedAt time.Time
}
