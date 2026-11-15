package follow

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/profile"
	"athlix/backend/internal/modules/user"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/userdir"
)

// Service implements follows and blocks.
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

// State is the response of POST / DELETE /users/:userId/follow.
type State struct {
	Following      bool `json:"following"`
	FollowersCount int  `json:"followersCount"`
}

func notSelf(me userdir.Basic, id int64) error {
	if me.ID == id {
		return httpx.Validation(map[string]string{"userId": "cannot be yourself"})
	}
	return nil
}

// target loads another user. Unknown and deleted users are 404; with active set, so are
// suspended users and users who have not finished onboarding. With checkBlocks, a block either
// way is also 404 (the same response, so blocks cannot be detected).
func (s *Service) target(ctx context.Context, me userdir.Basic, id int64, active, checkBlocks bool) (userdir.Basic, error) {
	t, err := s.d.Users.Get(ctx, id)
	if db.IsNoRows(err) {
		return t, httpx.NotFound("User")
	}
	if err != nil {
		return t, err
	}
	if t.Status == "deleted" || (active && (t.Status != "active" || t.OnboardingStep != user.StepDone)) {
		return t, httpx.NotFound("User")
	}
	if checkBlocks {
		blocked, err := s.d.Users.BlockedEitherWay(ctx, me.ID, id)
		if err != nil {
			return t, err
		}
		if blocked {
			return t, httpx.NotFound("User")
		}
	}
	return t, nil
}

// Follow is POST /users/:userId/follow (idempotent; follow.created only for a new follow).
func (s *Service) Follow(ctx context.Context, me userdir.Basic, id int64) (State, error) {
	if err := notSelf(me, id); err != nil {
		return State{}, err
	}
	if _, err := s.target(ctx, me, id, true, true); err != nil {
		return State{}, err
	}
	var added bool
	var count int
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		if added, err = s.repo.insertFollow(ctx, tx, me.ID, id); err != nil {
			return err
		}
		if added {
			if err := profile.AdjustFollowers(ctx, tx, id, 1); err != nil {
				return err
			}
			if err := profile.AdjustFollowing(ctx, tx, me.ID, 1); err != nil {
				return err
			}
		}
		count, err = profile.FollowersCount(ctx, tx, id)
		return err
	})
	if err != nil {
		return State{}, err
	}
	if !added {
		// Already following, or a block appeared since the check (then nothing was written).
		following, err := s.d.Users.Follows(ctx, me.ID, id)
		if err != nil {
			return State{}, err
		}
		if !following {
			return State{}, httpx.NotFound("User")
		}
		return State{Following: true, FollowersCount: count}, nil
	}
	ev := events.New(events.FollowCreated, me.ID, events.FollowPayload{FollowerID: me.ID, FolloweeID: id})
	if err := s.d.Events.Publish(ctx, ev); err != nil {
		logger.From(ctx).Error("publish follow.created", slog.Int64("followerId", me.ID), slog.Int64("followeeId", id), slog.String("err", err.Error()))
	}
	return State{Following: true, FollowersCount: count}, nil
}

// Unfollow is DELETE /users/:userId/follow (idempotent).
func (s *Service) Unfollow(ctx context.Context, me userdir.Basic, id int64) (State, error) {
	if err := notSelf(me, id); err != nil {
		return State{}, err
	}
	if _, err := s.target(ctx, me, id, false, true); err != nil {
		return State{}, err
	}
	var count int
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		removed, err := s.repo.deleteFollow(ctx, tx, me.ID, id)
		if err != nil {
			return err
		}
		if removed {
			if err := s.unlink(ctx, tx, me.ID, id); err != nil {
				return err
			}
		}
		count, err = profile.FollowersCount(ctx, tx, id)
		return err
	})
	if err != nil {
		return State{}, err
	}
	return State{Following: false, FollowersCount: count}, nil
}

// unlink decrements the counters after follower → followee was removed.
func (s *Service) unlink(ctx context.Context, q db.Querier, follower, followee int64) error {
	if err := profile.AdjustFollowers(ctx, q, followee, -1); err != nil {
		return err
	}
	return profile.AdjustFollowing(ctx, q, follower, -1)
}

// Block is POST /users/:userId/block: it also removes follows both ways (idempotent).
func (s *Service) Block(ctx context.Context, me userdir.Basic, id int64) error {
	if err := notSelf(me, id); err != nil {
		return err
	}
	if _, err := s.target(ctx, me, id, false, false); err != nil {
		return err
	}
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if err := s.repo.insertBlock(ctx, tx, me.ID, id); err != nil {
			return err
		}
		for _, pair := range [][2]int64{{me.ID, id}, {id, me.ID}} {
			removed, err := s.repo.deleteFollow(ctx, tx, pair[0], pair[1])
			if err != nil {
				return err
			}
			if removed {
				if err := s.unlink(ctx, tx, pair[0], pair[1]); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Unblock is DELETE /users/:userId/block (idempotent; only the caller's own block row).
func (s *Service) Unblock(ctx context.Context, me userdir.Basic, id int64) error {
	if err := notSelf(me, id); err != nil {
		return err
	}
	return s.repo.deleteBlock(ctx, me.ID, id)
}

// Followers is GET /users/:userId/followers.
func (s *Service) Followers(ctx context.Context, me userdir.Basic, id int64, page httpx.PageParams) ([]userdir.Summary, string, error) {
	return s.followList(ctx, kindFollowers, me, id, page)
}

// Following is GET /users/:userId/following.
func (s *Service) Following(ctx context.Context, me userdir.Basic, id int64, page httpx.PageParams) ([]userdir.Summary, string, error) {
	return s.followList(ctx, kindFollowing, me, id, page)
}

// followList applies the target checks and the profile visibility: a list the viewer may not
// see is an empty page.
func (s *Service) followList(ctx context.Context, kind listKind, me userdir.Basic, id int64, page httpx.PageParams) ([]userdir.Summary, string, error) {
	after, err := parseCursor(page.Cursor)
	if err != nil {
		return nil, "", err
	}
	if id != me.ID {
		if _, err := s.target(ctx, me, id, true, true); err != nil {
			return nil, "", err
		}
		ok, err := profile.ListsVisible(ctx, s.d, me.ID, id)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			return []userdir.Summary{}, "", nil
		}
	}
	return s.list(ctx, kind, me, id, after, page.Limit)
}

// Blocks is GET /me/blocks.
func (s *Service) Blocks(ctx context.Context, me userdir.Basic, page httpx.PageParams) ([]userdir.Summary, string, error) {
	after, err := parseCursor(page.Cursor)
	if err != nil {
		return nil, "", err
	}
	return s.list(ctx, kindBlocks, me, me.ID, after, page.Limit)
}

func parseCursor(c httpx.Cursor) (*entry, error) {
	if c.Time == "" && c.ID == 0 {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, c.Time)
	if err != nil || c.ID <= 0 {
		return nil, httpx.Validation(map[string]string{"cursor": "is invalid"})
	}
	return &entry{UserID: c.ID, At: t.UTC()}, nil
}

func (s *Service) list(ctx context.Context, kind listKind, me userdir.Basic, subject int64, after *entry, limit int) ([]userdir.Summary, string, error) {
	rows, err := s.repo.list(ctx, kind, subject, me.ID, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = httpx.EncodeCursor(httpx.Cursor{ID: last.UserID, Time: last.At.UTC().Format(time.RFC3339Nano)})
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	cards, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	out := []userdir.Summary{}
	for _, id := range ids {
		if c, ok := cards[id]; ok {
			out = append(out, c)
		}
	}
	if err := profile.HideMinorCities(ctx, s.d, me.ID, out); err != nil {
		return nil, "", err
	}
	return out, next, nil
}

// onUserDeleted removes every follow and block of a deleted user and fixes the other users'
// counters (idempotent: a second run finds nothing).
func (s *Service) onUserDeleted(ctx context.Context, e events.Event) error {
	var p events.UserPayload
	if err := e.Decode(&p); err != nil || p.UserID == 0 {
		return nil // malformed: nothing to retry
	}
	var followees, followers []int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		if followees, followers, err = s.repo.relations(ctx, tx, p.UserID); err != nil {
			return err
		}
		for _, id := range followees {
			if err := profile.AdjustFollowers(ctx, tx, id, -1); err != nil {
				return err
			}
		}
		for _, id := range followers {
			if err := profile.AdjustFollowing(ctx, tx, id, -1); err != nil {
				return err
			}
		}
		return s.repo.removeAll(ctx, tx, p.UserID)
	})
	if err != nil {
		return err
	}
	logger.From(ctx).Info("follows removed", slog.Int64("userId", p.UserID), slog.Int("following", len(followees)), slog.Int("followers", len(followers)))
	return nil
}
