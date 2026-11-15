package notification

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/testutil"
)

func TestUnreadCount(t *testing.T) {
	m, mock, _, _ := setup(t, nil)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM notifications WHERE user_id = \? AND read_at IS NULL`).WithArgs(me).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(4))
	w := testutil.Request(userServer(m), http.MethodGet, "/v1/notifications/unread-count", nil, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"unreadCount":4`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
