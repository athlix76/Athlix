package chat

import (
	"context"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

// privacy mocks user_settings.privacy.allowDmsFrom of the recipient ("" = no row).
func (e *env) privacy(recipient int64, v string) {
	r := sqlmock.NewRows([]string{"v"})
	if v != "" {
		r.AddRow(v)
	}
	e.mock.ExpectQuery(qPrivacy).WithArgs(recipient).WillReturnRows(r)
}

func TestCanMessageRules(t *testing.T) {
	const s, r = int64(1), int64(2)
	notDone := adult(r)
	notDone.OnboardingStep = "profile"

	cases := []struct {
		name      string
		cfg       map[string]any
		sender    userdir.Basic
		recipient userdir.Basic
		mocks     func(e *env)
		want      bool
	}{
		{"recipient onboarding not done", nil, adult(s), notDone, func(*env) {}, false},
		{"blocked either way", nil, adult(s), adult(r), func(e *env) {
			e.expectCount(qBlocks, 1, s, r, r, s)
		}, false},
		{"adults, default everyone", nil, adult(s), adult(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.privacy(r, "")
		}, true},
		{"followers: sender is followed", nil, adult(s), adult(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.privacy(r, "followers")
			e.expectCount(qFollows, 1, r, s)
		}, true},
		{"mutual: one way only, no consultation", nil, adult(s), adult(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.privacy(r, "mutual")
			e.expectCount(qMutual, 1)
			e.expectCount(qConsult, 0)
		}, false},
		{"nobody, no consultation", nil, adult(s), adult(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.privacy(r, "nobody")
			e.expectCount(qConsult, 0, s, r, r, s)
		}, false},
		{"consultation exception for adults", nil, adult(s), adult(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.privacy(r, "followers")
			e.expectCount(qFollows, 0)
			e.expectCount(qConsult, 1)
		}, true},
		{"dm_requires_follow turns everyone into followers", map[string]any{"chat.dm_requires_follow": true}, adult(s), adult(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.privacy(r, "everyone")
			e.expectCount(qFollows, 0, r, s)
			e.expectCount(qConsult, 0)
		}, false},
		{"dm_requires_follow with a follow", map[string]any{"chat.dm_requires_follow": true}, adult(s), adult(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.privacy(r, "")
			e.expectCount(qFollows, 1, r, s)
		}, true},
		{"minor recipient, mutual_follow, not mutual", nil, adult(s), minor(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.expectCount(qMutual, 1, s, r, r, s)
		}, false},
		{"minor sender, mutual_follow, mutual", nil, minor(s), adult(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.expectCount(qMutual, 2)
			e.privacy(r, "")
		}, true},
		{"minor: no consultation exception", nil, adult(s), minor(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.expectCount(qMutual, 2)
			e.privacy(r, "followers")
			e.expectCount(qFollows, 0)
		}, false},
		{"minor.dm_policy none", map[string]any{"minor.dm_policy": "none"}, minor(s), minor(r), func(e *env) {
			e.expectCount(qBlocks, 0)
		}, false},
		{"minor.dm_policy anyone", map[string]any{"minor.dm_policy": "anyone"}, adult(s), minor(r), func(e *env) {
			e.expectCount(qBlocks, 0)
			e.privacy(r, "")
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, c.cfg)
			c.mocks(e)
			got, err := e.s.CanMessage(context.Background(), c.sender, c.recipient)
			if err != nil || got != c.want {
				t.Fatalf("got %v err %v, want %v", got, err, c.want)
			}
		})
	}
}

func TestDirect(t *testing.T) {
	t.Run("creates the conversation", func(t *testing.T) {
		e := newEnv(t, nil)
		e.expectUser(adult(2))
		e.expectCount(qBlocks, 0)
		e.privacy(2, "")
		e.mock.ExpectQuery(`SELECT id FROM conversations WHERE direct_key = \?`).WithArgs("1:2").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		e.mock.ExpectBegin()
		e.mock.ExpectExec(`INSERT INTO conversations \(type, direct_key`).WithArgs("1:2").WillReturnResult(sqlmock.NewResult(10, 1))
		e.mock.ExpectExec(`INSERT INTO conversation_members`).WithArgs(int64(10), int64(1), int64(10), int64(2)).WillReturnResult(sqlmock.NewResult(0, 2))
		e.mock.ExpectCommit()
		e.mock.ExpectQuery(qConv).WithArgs(int64(1), int64(10)).WillReturnRows(row(convColumns,
			[]any{10, "direct", nil, nil, nil, 2, nil, "active", testNow, "1:2", "member", nil, 0}))
		e.expectSummaries(2)

		w := testutil.Request(e.userEngine(adult(1)), http.MethodPost, "/v1/chat/direct", map[string]any{"userId": 2}, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
		c := data(t, w)
		if c["id"].(float64) != 10 || c["type"] != "direct" || c["otherUser"].(map[string]any)["id"].(float64) != 2 {
			t.Fatalf("conversation %v", c)
		}
	})
	t.Run("refused by the rules is 403", func(t *testing.T) {
		e := newEnv(t, nil)
		e.expectUser(adult(2))
		e.expectCount(qBlocks, 1)
		w := testutil.Request(e.userEngine(adult(1)), http.MethodPost, "/v1/chat/direct", map[string]any{"userId": 2}, nil)
		if w.Code != http.StatusForbidden || errCode(t, w) != httpx.CodeForbidden {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
	t.Run("self is 422, unknown user is 404", func(t *testing.T) {
		e := newEnv(t, nil)
		r := e.userEngine(adult(1))
		if w := testutil.Request(r, http.MethodPost, "/v1/chat/direct", map[string]any{"userId": 1}, nil); w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("self %d", w.Code)
		}
		e.mock.ExpectQuery(qUser).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows(userColumns))
		if w := testutil.Request(r, http.MethodPost, "/v1/chat/direct", map[string]any{"userId": 5}, nil); w.Code != http.StatusNotFound {
			t.Fatalf("unknown %d %s", w.Code, w.Body)
		}
	})
}
