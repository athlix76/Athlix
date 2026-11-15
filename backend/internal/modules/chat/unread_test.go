package chat

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/testutil"
)

func TestUnreadCountSkipsMutedAndUsesCaller(t *testing.T) {
	e := newEnv(t, nil)
	r := e.userEngine(testutil.VerifiedUser(7))
	e.mock.ExpectQuery(`SELECT COUNT\(\*\), COALESCE\(SUM\(t.unread\), 0\) FROM .*m.user_id = \? AND m.status = 'active'.*m.muted_until IS NULL OR m.muted_until <= \?`).
		WithArgs(int64(7), testNow).WillReturnRows(sqlmock.NewRows([]string{"c", "m"}).AddRow(2, 9))
	w := testutil.Request(r, http.MethodGet, "/v1/chat/unread-count", nil, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"conversations":2`) || !strings.Contains(w.Body.String(), `"messages":9`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if err := e.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
