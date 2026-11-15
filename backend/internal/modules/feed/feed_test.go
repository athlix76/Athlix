package feed

import (
	"context"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

// ---- helpers ----

var created = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

var postColumns = []string{"id", "author_id", "body", "media_upload_ids", "sport_id", "event_id", "visibility", "like_count", "comment_count", "created_at"}

type pr struct {
	id, author int64
	body       any // string or nil
	media      string
	sport      any // int64 or nil
	vis        string
	likes      int64
	comments   int64
}

func postRows(ps ...pr) *sqlmock.Rows {
	r := sqlmock.NewRows(postColumns)
	for _, p := range ps {
		media := p.media
		if media == "" {
			media = "[]"
		}
		vis := p.vis
		if vis == "" {
			vis = "public"
		}
		r.AddRow(p.id, p.author, p.body, []byte(media), p.sport, nil, vis, p.likes, p.comments, created)
	}
	return r
}

func summaryRows(ids ...int64) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"id", "display_name", "primary_role", "verification_status", "photo_upload_id", "city", "state", "s.id", "s.code", "s.name", "sp.id", "sp.code", "sp.name"})
	for _, id := range ids {
		r.AddRow(id, "User", "player", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil)
	}
	return r
}

func viewerArgs(v int64) []driver.Value { return []driver.Value{v, v, v, v} }

func args(head []driver.Value, tail ...driver.Value) []driver.Value {
	return append(append([]driver.Value{}, head...), tail...)
}

const (
	qVisiblePost = `FROM posts p WHERE p\.id = \? AND p\.status = 'visible'`
	qSummaries   = `FROM users u\s+LEFT JOIN profiles p`
	qLikedBy     = `SELECT post_id FROM post_likes WHERE user_id = \? AND post_id IN`
	qSports      = `SELECT id, code, name FROM sports WHERE id IN`
	qMediaView   = `SELECT id, s3_key FROM media_uploads WHERE status IN`
	qMediaGet    = `SELECT id, user_id, purpose, s3_key, content_type, size_bytes, status FROM media_uploads`
	qInsertPost  = `INSERT INTO posts`
)

func setup(t *testing.T, u userdir.Basic, cfg map[string]any) (*app.Deps, sqlmock.Sqlmock, http.Handler) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return created }
	r, rt := testutil.UserEngine(u)
	New(d).Routes(rt)
	return d, mock, r
}

type envelope struct {
	Data  json.RawMessage             `json:"data"`
	Meta  struct{ NextCursor string } `json:"meta"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func decode(t *testing.T, body []byte) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("bad json %s: %v", body, err)
	}
	return e
}

func expectStatus(t *testing.T, code, want int, body string) {
	t.Helper()
	if code != want {
		t.Fatalf("status = %d, want %d; body %s", code, want, body)
	}
}

// recorder registers a handler for typ that records events and whether every SQL
// expectation (including the commit) had already been met when it ran.
type recorder struct {
	mu         sync.Mutex
	got        []events.PostPayload
	afterWrite []bool
}

func record(d *app.Deps, mock sqlmock.Sqlmock, typ string) *recorder {
	rec := &recorder{}
	d.Dispatcher.On(typ, func(_ context.Context, e events.Event) error {
		var p events.PostPayload
		_ = e.Decode(&p)
		rec.mu.Lock()
		rec.got = append(rec.got, p)
		rec.afterWrite = append(rec.afterWrite, mock.ExpectationsWereMet() == nil)
		rec.mu.Unlock()
		return nil
	})
	return rec
}

// ---- GET /posts/:id and visibility ----

func TestGetPostHappyPath(t *testing.T) {
	_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	mock.ExpectQuery(qVisiblePost).WithArgs(args([]driver.Value{int64(70)}, viewerArgs(1)...)...).
		WillReturnRows(postRows(pr{id: 70, author: 2, body: "hello", media: "[5]", sport: int64(3), likes: 2, comments: 1}))
	mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(2))
	mock.ExpectQuery(qSports).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name"}).AddRow(3, "cricket", "Cricket"))
	mock.ExpectQuery(qMediaView).WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(5, "post_image/2/a.jpg"))
	mock.ExpectQuery(qLikedBy).WithArgs(int64(1), int64(70)).WillReturnRows(sqlmock.NewRows([]string{"post_id"}).AddRow(70))

	w := testutil.Request(r, "GET", "/v1/posts/70", nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	var p map[string]any
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &p)
	if p["body"] != "hello" || p["likedByMe"] != true || p["isOwn"] != false || p["eventId"] != nil || p["visibility"] != "public" {
		t.Fatalf("unexpected post %v", p)
	}
	if imgs := p["images"].([]any); len(imgs) != 1 || imgs[0] != "https://storage.test/get/post_image/2/a.jpg" {
		t.Fatalf("images = %v", p["images"])
	}
	if sp := p["sport"].(map[string]any); sp["code"] != "cricket" || sp["name"] != "Cricket" {
		t.Fatalf("sport = %v", sp)
	}
	if p["likeCount"].(float64) != 2 || p["commentCount"].(float64) != 1 {
		t.Fatalf("counts = %v", p)
	}
	for _, k := range []string{"id", "author", "body", "images", "sport", "eventId", "visibility", "likeCount", "commentCount", "likedByMe", "isOwn", "createdAt"} {
		if _, ok := p[k]; !ok {
			t.Fatalf("missing key %s", k)
		}
	}
}

// The visibility rule lives in SQL; these cases prove the query enforces follow and block
// checks and that "no row" is a 404 (followers-only post of a non-followed user, blocked
// author, hidden/deleted post).
func TestGetPostNotVisibleIs404(t *testing.T) {
	cases := map[string]string{
		"followers-only, not followed": `p\.visibility = 'public' OR p\.author_id = \? OR EXISTS \(SELECT 1 FROM follows f WHERE f\.follower_id = \?`,
		"blocked author":               `NOT EXISTS \(SELECT 1 FROM blocks b WHERE \(b\.blocker_id = \? AND b\.blocked_id = p\.author_id\) OR \(b\.blocker_id = p\.author_id AND b\.blocked_id = \?\)\)`,
		"inactive author":              `EXISTS \(SELECT 1 FROM users u WHERE u\.id = p\.author_id AND u\.status = 'active'\)`,
	}
	for name, clause := range cases {
		t.Run(name, func(t *testing.T) {
			_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
			mock.ExpectQuery(qVisiblePost + `[\s\S]*` + clause).WithArgs(args([]driver.Value{int64(70)}, viewerArgs(1)...)...).
				WillReturnRows(sqlmock.NewRows(postColumns))
			w := testutil.Request(r, "GET", "/v1/posts/70", nil, nil)
			expectStatus(t, w.Code, 404, w.Body.String())
		})
	}
}

// ---- DELETE /posts/:id ----

func TestDeletePost(t *testing.T) {
	t.Run("own post", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
		mock.ExpectExec(`UPDATE posts SET status = 'deleted' WHERE id = \? AND author_id = \? AND status = 'visible'`).
			WithArgs(int64(70), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
		w := testutil.Request(r, "DELETE", "/v1/posts/70", nil, nil)
		expectStatus(t, w.Code, 204, w.Body.String())
	})
	t.Run("someone else's post is 404", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
		mock.ExpectExec(`UPDATE posts SET status = 'deleted' WHERE id = \? AND author_id = \?`).
			WithArgs(int64(70), int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
		w := testutil.Request(r, "DELETE", "/v1/posts/70", nil, nil)
		expectStatus(t, w.Code, 404, w.Body.String())
	})
}

// ---- POST /posts ----

func expectTextPost(mock sqlmock.Sqlmock, author int64, vis string) {
	mock.ExpectBegin()
	mock.ExpectExec(qInsertPost).WithArgs(author, sqlmock.AnyArg(), "[]", nil, nil, vis).WillReturnResult(sqlmock.NewResult(80, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(author))
}

func TestCreatePostBodyLengthFollowsSetting(t *testing.T) {
	body := map[string]any{"body": "  123456  "}
	t.Run("limit 5 rejects", func(t *testing.T) {
		_, _, r := setup(t, testutil.VerifiedUser(1), map[string]any{"feed.post_max_length": 5})
		w := testutil.Request(r, "POST", "/v1/posts", body, nil)
		expectStatus(t, w.Code, 422, w.Body.String())
		if decode(t, w.Body.Bytes()).Error.Fields["body"] == "" {
			t.Fatalf("expected body field error: %s", w.Body.String())
		}
	})
	t.Run("limit 10 accepts", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), map[string]any{"feed.post_max_length": 10})
		expectTextPost(mock, 1, "public")
		w := testutil.Request(r, "POST", "/v1/posts", body, nil)
		expectStatus(t, w.Code, 200, w.Body.String())
		var p Post
		_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &p)
		if p.ID != 80 || p.Body == nil || *p.Body != "123456" || !p.IsOwn || p.LikedByMe || len(p.Images) != 0 || p.Images == nil {
			t.Fatalf("unexpected post %+v", p)
		}
	})
}

func TestCreatePostMediaCountFollowsSetting(t *testing.T) {
	body := map[string]any{"mediaUploadIds": []int64{11, 12}}
	t.Run("max 1 rejects", func(t *testing.T) {
		_, _, r := setup(t, testutil.VerifiedUser(1), map[string]any{"feed.post_max_media": 1})
		w := testutil.Request(r, "POST", "/v1/posts", body, nil)
		expectStatus(t, w.Code, 422, w.Body.String())
		if decode(t, w.Body.Bytes()).Error.Fields["mediaUploadIds"] == "" {
			t.Fatalf("expected mediaUploadIds error: %s", w.Body.String())
		}
	})
	t.Run("max 2 accepts and claims", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), map[string]any{"feed.post_max_media": 2})
		mock.ExpectBegin()
		for _, id := range []int64{11, 12} {
			mock.ExpectQuery(qMediaGet).WithArgs(id, int64(1)).WillReturnRows(
				sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"}).
					AddRow(id, 1, "post_image", "post_image/1/k"+string(rune('0'+id-10))+".jpg", "image/jpeg", 100, "confirmed"))
			mock.ExpectExec(`UPDATE media_uploads SET status = 'attached'`).WithArgs(id, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectExec(qInsertPost).WithArgs(int64(1), nil, "[11,12]", nil, nil, "public").WillReturnResult(sqlmock.NewResult(81, 1))
		mock.ExpectCommit()
		mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(1))
		mock.ExpectQuery(qMediaView).WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(11, "post_image/1/k1.jpg").AddRow(12, "post_image/1/k2.jpg"))
		w := testutil.Request(r, "POST", "/v1/posts", body, nil)
		expectStatus(t, w.Code, 200, w.Body.String())
		var p Post
		_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &p)
		if p.Body != nil || len(p.Images) != 2 || p.Images[0] != "https://storage.test/get/post_image/1/k1.jpg" {
			t.Fatalf("unexpected post %+v", p)
		}
	})
}

func TestCreatePostValidation(t *testing.T) {
	cases := map[string]struct {
		body  map[string]any
		field string
	}{
		"empty post":      {map[string]any{"body": "   "}, "body"},
		"duplicate media": {map[string]any{"mediaUploadIds": []int64{4, 4}}, "mediaUploadIds"},
		"non-positive id": {map[string]any{"mediaUploadIds": []int64{0}}, "mediaUploadIds"},
		"bad visibility":  {map[string]any{"body": "x", "visibility": "everyone"}, "visibility"},
		"bad sport id":    {map[string]any{"body": "x", "sportId": -1}, "sportId"},
		"bad event id":    {map[string]any{"body": "x", "eventId": 0}, "eventId"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, r := setup(t, testutil.VerifiedUser(1), nil)
			w := testutil.Request(r, "POST", "/v1/posts", tc.body, nil)
			expectStatus(t, w.Code, 422, w.Body.String())
			if decode(t, w.Body.Bytes()).Error.Fields[tc.field] == "" {
				t.Fatalf("expected %s error: %s", tc.field, w.Body.String())
			}
		})
	}
}

func TestCreatePostSportAndEventChecks(t *testing.T) {
	_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM sports WHERE id = \? AND active = 1`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM events WHERE id = \? AND status IN \('published','completed'\)`).WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	w := testutil.Request(r, "POST", "/v1/posts", map[string]any{"body": "x", "sportId": 3, "eventId": 9}, nil)
	expectStatus(t, w.Code, 422, w.Body.String())
	f := decode(t, w.Body.Bytes()).Error.Fields
	if f["sportId"] == "" || f["eventId"] == "" {
		t.Fatalf("fields = %v", f)
	}
}

func TestCreatePostUnclaimableMediaIs422(t *testing.T) {
	_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	mock.ExpectBegin()
	mock.ExpectQuery(qMediaGet).WithArgs(int64(11), int64(1)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"}))
	mock.ExpectRollback()
	w := testutil.Request(r, "POST", "/v1/posts", map[string]any{"mediaUploadIds": []int64{11}}, nil)
	expectStatus(t, w.Code, 422, w.Body.String())
	if decode(t, w.Body.Bytes()).Error.Fields["mediaUploadIds"] == "" {
		t.Fatalf("expected mediaUploadIds error: %s", w.Body.String())
	}
}

func TestCreatePostMinorVisibility(t *testing.T) {
	minor := testutil.VerifiedUser(1)
	minor.IsMinor = true
	t.Run("defaults to followers", func(t *testing.T) {
		_, mock, r := setup(t, minor, nil)
		expectTextPost(mock, 1, "followers")
		w := testutil.Request(r, "POST", "/v1/posts", map[string]any{"body": "hi"}, nil)
		expectStatus(t, w.Code, 200, w.Body.String())
		var p Post
		_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &p)
		if p.Visibility != "followers" {
			t.Fatalf("visibility = %s", p.Visibility)
		}
	})
	t.Run("public rejected", func(t *testing.T) {
		_, _, r := setup(t, minor, nil)
		w := testutil.Request(r, "POST", "/v1/posts", map[string]any{"body": "hi", "visibility": "public"}, nil)
		expectStatus(t, w.Code, 422, w.Body.String())
		if decode(t, w.Body.Bytes()).Error.Fields["visibility"] == "" {
			t.Fatalf("expected visibility error: %s", w.Body.String())
		}
	})
	t.Run("setting public allows public", func(t *testing.T) {
		_, mock, r := setup(t, minor, map[string]any{"minor.profile_default_visibility": "public"})
		expectTextPost(mock, 1, "public")
		w := testutil.Request(r, "POST", "/v1/posts", map[string]any{"body": "hi"}, nil)
		expectStatus(t, w.Code, 200, w.Body.String())
	})
	t.Run("adult defaults to public", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
		expectTextPost(mock, 1, "public")
		w := testutil.Request(r, "POST", "/v1/posts", map[string]any{"body": "hi"}, nil)
		expectStatus(t, w.Code, 200, w.Body.String())
	})
}

// ---- likes ----

func expectLike(mock sqlmock.Sqlmock, inserted bool, author, count int64) {
	mock.ExpectBegin()
	mock.ExpectQuery(qVisiblePost).WithArgs(args([]driver.Value{int64(70)}, viewerArgs(1)...)...).WillReturnRows(postRows(pr{id: 70, author: author, body: "x"}))
	if inserted {
		mock.ExpectExec(`INSERT IGNORE INTO post_likes`).WithArgs(int64(70), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE posts SET like_count = like_count \+ 1 WHERE id = \?`).WithArgs(int64(70)).WillReturnResult(sqlmock.NewResult(0, 1))
	} else {
		mock.ExpectExec(`INSERT IGNORE INTO post_likes`).WithArgs(int64(70), int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectQuery(`SELECT like_count FROM posts WHERE id = \?`).WithArgs(int64(70)).WillReturnRows(sqlmock.NewRows([]string{"like_count"}).AddRow(count))
	mock.ExpectCommit()
}

func TestLikeIsIdempotentAndPublishesAfterCommit(t *testing.T) {
	d, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	rec := record(d, mock, events.PostLiked)

	expectLike(mock, true, 2, 1)
	w := testutil.Request(r, "POST", "/v1/posts/70/like", nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	var res LikeResult
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &res)
	if !res.Liked || res.LikeCount != 1 {
		t.Fatalf("first like = %+v", res)
	}

	expectLike(mock, false, 2, 1) // second like: no counter update
	w = testutil.Request(r, "POST", "/v1/posts/70/like", nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &res)
	if !res.Liked || res.LikeCount != 1 {
		t.Fatalf("second like = %+v", res)
	}

	if len(rec.got) != 1 {
		t.Fatalf("published %d post.liked events, want 1", len(rec.got))
	}
	if got := rec.got[0]; got.PostID != 70 || got.AuthorID != 2 || got.ActorID != 1 {
		t.Fatalf("payload = %+v", got)
	}
	if !rec.afterWrite[0] {
		t.Fatal("post.liked was published before the transaction committed")
	}
}

func TestLikeOwnPostDoesNotPublish(t *testing.T) {
	d, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	rec := record(d, mock, events.PostLiked)
	expectLike(mock, true, 1, 1)
	w := testutil.Request(r, "POST", "/v1/posts/70/like", nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	if len(rec.got) != 0 {
		t.Fatalf("self like published %d events", len(rec.got))
	}
}

func TestLikeInvisiblePostIs404(t *testing.T) {
	_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	mock.ExpectBegin()
	mock.ExpectQuery(qVisiblePost).WillReturnRows(sqlmock.NewRows(postColumns))
	mock.ExpectRollback()
	w := testutil.Request(r, "POST", "/v1/posts/70/like", nil, nil)
	expectStatus(t, w.Code, 404, w.Body.String())
}

func TestUnlike(t *testing.T) {
	_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	mock.ExpectBegin()
	mock.ExpectQuery(qVisiblePost).WillReturnRows(postRows(pr{id: 70, author: 2, body: "x", likes: 1}))
	mock.ExpectExec(`DELETE FROM post_likes WHERE post_id = \? AND user_id = \?`).WithArgs(int64(70), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE posts SET like_count = IF\(like_count > 0, like_count - 1, 0\) WHERE id = \?`).WithArgs(int64(70)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT like_count FROM posts`).WillReturnRows(sqlmock.NewRows([]string{"like_count"}).AddRow(0))
	mock.ExpectCommit()
	w := testutil.Request(r, "DELETE", "/v1/posts/70/like", nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	var res LikeResult
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &res)
	if res.Liked || res.LikeCount != 0 {
		t.Fatalf("unlike = %+v", res)
	}
}

// ---- comments ----

func TestListCommentsOldestFirstWithCursor(t *testing.T) {
	_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	mock.ExpectQuery(qVisiblePost).WillReturnRows(postRows(pr{id: 70, author: 2, body: "x"}))
	mock.ExpectQuery(`FROM post_comments c[\s\S]*c\.id > \?[\s\S]*ORDER BY c\.id ASC LIMIT \?`).
		WithArgs(int64(70), int64(0), int64(1), int64(1), 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "post_id", "author_id", "body", "created_at"}).
			AddRow(5, 70, 1, "first", created).AddRow(6, 70, 2, "second", created))
	mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(1))
	w := testutil.Request(r, "GET", "/v1/posts/70/comments?limit=1", nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	e := decode(t, w.Body.Bytes())
	var items []Comment
	_ = json.Unmarshal(e.Data, &items)
	if len(items) != 1 || items[0].ID != 5 || !items[0].IsOwn || items[0].Body != "first" || items[0].PostID != 70 {
		t.Fatalf("items = %+v", items)
	}
	if cur := cursorOf(t, e.Meta.NextCursor); cur.ID != 5 {
		t.Fatalf("next cursor = %+v", cur)
	}
}

func TestAddCommentPublishesAfterCommit(t *testing.T) {
	d, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	rec := record(d, mock, events.PostCommented)
	mock.ExpectBegin()
	mock.ExpectQuery(qVisiblePost).WillReturnRows(postRows(pr{id: 70, author: 2, body: "x"}))
	mock.ExpectExec(`INSERT INTO post_comments \(post_id, author_id, body\) VALUES`).WithArgs(int64(70), int64(1), "nice").WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(`UPDATE posts SET comment_count = comment_count \+ 1 WHERE id = \?`).WithArgs(int64(70)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(1))
	w := testutil.Request(r, "POST", "/v1/posts/70/comments", map[string]any{"body": "  nice "}, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	var cm Comment
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &cm)
	if cm.ID != 9 || cm.Body != "nice" || !cm.IsOwn || cm.Author.ID != 1 {
		t.Fatalf("comment = %+v", cm)
	}
	if len(rec.got) != 1 || rec.got[0].CommentID != 9 || rec.got[0].AuthorID != 2 || rec.got[0].ActorID != 1 {
		t.Fatalf("events = %+v", rec.got)
	}
	if !rec.afterWrite[0] {
		t.Fatal("post.commented was published before the transaction committed")
	}
}

func TestAddCommentOnOwnPostDoesNotPublish(t *testing.T) {
	d, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	rec := record(d, mock, events.PostCommented)
	mock.ExpectBegin()
	mock.ExpectQuery(qVisiblePost).WillReturnRows(postRows(pr{id: 70, author: 1, body: "x"}))
	mock.ExpectExec(`INSERT INTO post_comments`).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(`UPDATE posts SET comment_count`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(1))
	w := testutil.Request(r, "POST", "/v1/posts/70/comments", map[string]any{"body": "me"}, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	if len(rec.got) != 0 {
		t.Fatalf("self comment published %d events", len(rec.got))
	}
}

func TestAddCommentLengthFromConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  map[string]any
		body string
	}{
		{"configured 5", map[string]any{"feed.comment_max_length": 5}, "abcdef"},
		{"clamped to the column", map[string]any{"feed.comment_max_length": 5000}, strings.Repeat("a", commentBodyColumnMax+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, r := setup(t, testutil.VerifiedUser(1), tc.cfg)
			w := testutil.Request(r, "POST", "/v1/posts/70/comments", map[string]any{"body": tc.body}, nil)
			expectStatus(t, w.Code, 422, w.Body.String())
		})
	}
}

func TestAddCommentValidation(t *testing.T) {
	for name, body := range map[string]string{"empty": "   ", "too long": strings.Repeat("a", commentBodyColumnMax+1)} {
		t.Run(name, func(t *testing.T) {
			_, _, r := setup(t, testutil.VerifiedUser(1), nil)
			w := testutil.Request(r, "POST", "/v1/posts/70/comments", map[string]any{"body": body}, nil)
			expectStatus(t, w.Code, 422, w.Body.String())
			if decode(t, w.Body.Bytes()).Error.Fields["body"] == "" {
				t.Fatalf("expected body error: %s", w.Body.String())
			}
		})
	}
}

func TestDeleteComment(t *testing.T) {
	const q = `SELECT c\.post_id FROM post_comments c JOIN posts p ON p\.id = c\.post_id[\s\S]*\(c\.author_id = \? OR p\.author_id = \?\) FOR UPDATE`
	t.Run("author or post author", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
		mock.ExpectBegin()
		mock.ExpectQuery(q).WithArgs(int64(9), int64(1), int64(1)).WillReturnRows(sqlmock.NewRows([]string{"post_id"}).AddRow(70))
		mock.ExpectExec(`UPDATE post_comments SET status = 'deleted' WHERE id = \?`).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE posts SET comment_count = IF\(comment_count > 0, comment_count - 1, 0\) WHERE id = \?`).WithArgs(int64(70)).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		w := testutil.Request(r, "DELETE", "/v1/comments/9", nil, nil)
		expectStatus(t, w.Code, 204, w.Body.String())
	})
	t.Run("anyone else is 404", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
		mock.ExpectBegin()
		mock.ExpectQuery(q).WithArgs(int64(9), int64(1), int64(1)).WillReturnRows(sqlmock.NewRows([]string{"post_id"}))
		mock.ExpectRollback()
		w := testutil.Request(r, "DELETE", "/v1/comments/9", nil, nil)
		expectStatus(t, w.Code, 404, w.Body.String())
	})
}

// ---- feeds ----

func cursorOf(t *testing.T, s string) struct {
	ID   int64  `json:"i"`
	Time string `json:"t"`
} {
	t.Helper()
	var c struct {
		ID   int64  `json:"i"`
		Time string `json:"t"`
	}
	if s == "" {
		t.Fatal("expected a next cursor")
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || json.Unmarshal(b, &c) != nil {
		t.Fatalf("bad cursor %q", s)
	}
	return c
}

func TestFollowingFeed(t *testing.T) {
	_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	mock.ExpectQuery(`WHERE \(p\.author_id = \? OR EXISTS \(SELECT 1 FROM follows f2[\s\S]*ORDER BY p\.id DESC LIMIT \?`).
		WithArgs(args([]driver.Value{int64(1), int64(1)}, append(viewerArgs(1), 21)...)...).
		WillReturnRows(postRows(pr{id: 72, author: 1, body: "mine"}, pr{id: 71, author: 3, body: "friend", vis: "followers"}))
	mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(1, 3))
	mock.ExpectQuery(qLikedBy).WillReturnRows(sqlmock.NewRows([]string{"post_id"}))
	w := testutil.Request(r, "GET", "/v1/feed", nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	e := decode(t, w.Body.Bytes())
	var posts []Post
	_ = json.Unmarshal(e.Data, &posts)
	if len(posts) != 2 || posts[0].ID != 72 || !posts[0].IsOwn || posts[1].IsOwn || e.Meta.NextCursor != "" {
		t.Fatalf("posts = %+v meta = %+v", posts, e.Meta)
	}
}

func TestFeedLimitOnlyLowersPageSize(t *testing.T) {
	_, mock, r := setup(t, testutil.VerifiedUser(1), map[string]any{"feed.page_size": 1})
	mock.ExpectQuery(`FROM follows f2`).
		WithArgs(args([]driver.Value{int64(1), int64(1)}, append(viewerArgs(1), 2)...)...). // limit+1 = 2, not 51
		WillReturnRows(postRows(pr{id: 72, author: 1, body: "a"}, pr{id: 71, author: 1, body: "b"}))
	mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(1))
	mock.ExpectQuery(qLikedBy).WillReturnRows(sqlmock.NewRows([]string{"post_id"}))
	w := testutil.Request(r, "GET", "/v1/feed?mode=following&limit=50", nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	e := decode(t, w.Body.Bytes())
	var posts []Post
	_ = json.Unmarshal(e.Data, &posts)
	if len(posts) != 1 || cursorOf(t, e.Meta.NextCursor).ID != 72 {
		t.Fatalf("posts = %+v meta = %+v", posts, e.Meta)
	}
}

func TestFeedBadModeIs422(t *testing.T) {
	_, _, r := setup(t, testutil.VerifiedUser(1), nil)
	w := testutil.Request(r, "GET", "/v1/feed?mode=trending", nil, nil)
	expectStatus(t, w.Code, 422, w.Body.String())
	if decode(t, w.Body.Bytes()).Error.Fields["mode"] == "" {
		t.Fatalf("expected mode error: %s", w.Body.String())
	}
}

func TestDiscoverMySportsFirstThenOthers(t *testing.T) {
	_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
	mock.ExpectQuery(`SELECT sport_id FROM profile_sports WHERE user_id = \?\s+UNION SELECT sport_id FROM profiles`).
		WithArgs(int64(1), int64(1)).WillReturnRows(sqlmock.NewRows([]string{"sport_id"}).AddRow(3))
	mock.ExpectQuery(`p\.visibility = 'public' AND[\s\S]*AND p\.sport_id IN \(\?\) ORDER BY p\.id DESC LIMIT \?`).
		WithArgs(args(viewerArgs(1), int64(3), 3)...).
		WillReturnRows(postRows(pr{id: 50, author: 2, body: "cricket", sport: int64(3)}))
	mock.ExpectQuery(`AND \(p\.sport_id IS NULL OR p\.sport_id NOT IN \(\?\)\) ORDER BY p\.id DESC LIMIT \?`).
		WithArgs(args(viewerArgs(1), int64(3), 2)...).
		WillReturnRows(postRows(pr{id: 90, author: 4, body: "other"}, pr{id: 80, author: 4, body: "older"}))
	mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(2, 4))
	mock.ExpectQuery(qSports).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name"}).AddRow(3, "cricket", "Cricket"))
	mock.ExpectQuery(qLikedBy).WillReturnRows(sqlmock.NewRows([]string{"post_id"}))
	w := testutil.Request(r, "GET", "/v1/feed?mode=discover&limit=2", nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	e := decode(t, w.Body.Bytes())
	var posts []Post
	_ = json.Unmarshal(e.Data, &posts)
	if len(posts) != 2 || posts[0].ID != 50 || posts[1].ID != 90 {
		t.Fatalf("posts = %+v", posts)
	}
	cur := cursorOf(t, e.Meta.NextCursor)
	if cur.ID != 90 || cur.Time != "o" {
		t.Fatalf("cursor = %+v", cur)
	}

	// The next page continues in the "other" phase below id 90.
	mock.ExpectQuery(`SELECT sport_id FROM profile_sports`).WillReturnRows(sqlmock.NewRows([]string{"sport_id"}).AddRow(3))
	mock.ExpectQuery(`p\.sport_id NOT IN \(\?\)\) AND p\.id < \? ORDER BY p\.id DESC LIMIT \?`).
		WithArgs(args(viewerArgs(1), int64(3), int64(90), 3)...).
		WillReturnRows(postRows(pr{id: 80, author: 4, body: "older"}))
	mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(4))
	mock.ExpectQuery(qLikedBy).WillReturnRows(sqlmock.NewRows([]string{"post_id"}))
	w = testutil.Request(r, "GET", "/v1/feed?mode=discover&limit=2&cursor="+e.Meta.NextCursor, nil, nil)
	expectStatus(t, w.Code, 200, w.Body.String())
	e = decode(t, w.Body.Bytes())
	_ = json.Unmarshal(e.Data, &posts)
	if len(posts) != 1 || posts[0].ID != 80 || e.Meta.NextCursor != "" {
		t.Fatalf("page 2 = %+v meta = %+v", posts, e.Meta)
	}
}

// ---- GET /users/:userId/posts ----

func TestUserPosts(t *testing.T) {
	const qActive = `SELECT COUNT\(\*\) FROM users WHERE id = \? AND status = 'active'`
	const qBlocks = `SELECT COUNT\(\*\) FROM blocks WHERE`
	t.Run("happy path", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
		mock.ExpectQuery(qActive).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
		mock.ExpectQuery(qBlocks).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
		mock.ExpectQuery(`FROM posts p WHERE p\.author_id = \? AND`).WithArgs(args([]driver.Value{int64(2)}, append(viewerArgs(1), 21)...)...).
			WillReturnRows(postRows(pr{id: 70, author: 2, body: "x"}))
		mock.ExpectQuery(qSummaries).WillReturnRows(summaryRows(2))
		mock.ExpectQuery(qLikedBy).WillReturnRows(sqlmock.NewRows([]string{"post_id"}))
		w := testutil.Request(r, "GET", "/v1/users/2/posts", nil, nil)
		expectStatus(t, w.Code, 200, w.Body.String())
		var posts []Post
		_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &posts)
		if len(posts) != 1 || posts[0].Author.ID != 2 {
			t.Fatalf("posts = %+v", posts)
		}
	})
	t.Run("blocked is 404", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
		mock.ExpectQuery(qActive).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
		mock.ExpectQuery(qBlocks).WithArgs(int64(1), int64(2), int64(2), int64(1)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
		w := testutil.Request(r, "GET", "/v1/users/2/posts", nil, nil)
		expectStatus(t, w.Code, 404, w.Body.String())
	})
	t.Run("inactive user is 404", func(t *testing.T) {
		_, mock, r := setup(t, testutil.VerifiedUser(1), nil)
		mock.ExpectQuery(qActive).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
		w := testutil.Request(r, "GET", "/v1/users/2/posts", nil, nil)
		expectStatus(t, w.Code, 404, w.Body.String())
	})
}
