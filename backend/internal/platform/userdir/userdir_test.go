package userdir

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSummariesCarrySportAndPositionCode(t *testing.T) {
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mock.ExpectQuery(`s\.id, s\.code, s\.name, sp\.id, sp\.code, sp\.name\s+FROM users u`).WithArgs(int64(7), int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "display_name", "primary_role", "verification_status", "photo_upload_id",
			"city", "state", "s.id", "s.code", "s.name", "sp.id", "sp.code", "sp.name"}).
			AddRow(7, "Asha", "player", "verified", nil, "Pune", nil, 1, "cricket", "Cricket", 3, "batter", "Batter").
			AddRow(8, "Ravi", "coach", "unverified", nil, nil, nil, nil, nil, nil, nil, nil, nil))
	got, err := New(conn, nil).Summaries(context.Background(), []int64{7, 8})
	if err != nil {
		t.Fatal(err)
	}
	a := got[7]
	if a.Sport == nil || a.Sport.Code != "cricket" || a.Sport.Name != "Cricket" || a.Position == nil || a.Position.Code != "batter" {
		t.Fatalf("summary %+v", a)
	}
	if !a.IsVerified || a.City == nil || *a.City != "Pune" {
		t.Fatalf("summary %+v", a)
	}
	b, _ := json.Marshal(a.Sport)
	if string(b) != `{"id":1,"code":"cricket","name":"Cricket"}` {
		t.Fatalf("json %s", b)
	}
	if r := got[8]; r.Sport != nil || r.Position != nil || r.IsVerified {
		t.Fatalf("summary %+v", r)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
