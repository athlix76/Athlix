package locationidentity

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestImportDistricts(t *testing.T) {
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	csv := "state_code,district_name,lgd_code,aliases\n" +
		"TN,Thanjavur,600,Tanjore;Tanjavur\n" + // line 2: update by lgd_code, aliases replaced
		"TN,Mayiladuthurai,640,Mayavaram\n" + // line 3: new district
		"XX,Nowhere,999\n" + // line 4: unknown state
		"tn,Coimbatore,,Kovai\n" + // line 5: no lgd_code, update by (state, name)
		"TN,,5\n" + // line 6: missing name
		"TN,Ariyalur,abc\n" // line 7: bad lgd_code

	const byLgd = `SELECT id FROM districts WHERE lgd_code = \?`
	const byName = `SELECT id FROM districts WHERE state_id = \? AND name = \?`
	const update = `UPDATE districts SET state_id = \?, name = \?, lgd_code = COALESCE\(\?, lgd_code\),\s+aliases = COALESCE\(\?, aliases\) WHERE id = \?`
	idRows := func(ids ...int64) *sqlmock.Rows {
		r := sqlmock.NewRows([]string{"id"})
		for _, id := range ids {
			r.AddRow(id)
		}
		return r
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, code FROM states`).WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(stateTN, "TN").AddRow(12, "KL"))
	mock.ExpectQuery(byLgd).WithArgs(600).WillReturnRows(idRows(thanjavur))
	mock.ExpectExec(update).WithArgs(stateTN, "Thanjavur", 600, `["Tanjore","Tanjavur"]`, thanjavur).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(byLgd).WithArgs(640).WillReturnRows(idRows())
	mock.ExpectQuery(byName).WithArgs(stateTN, "Mayiladuthurai").WillReturnRows(idRows())
	mock.ExpectExec(`INSERT INTO districts \(state_id, name, lgd_code, aliases, active\) VALUES \(\?, \?, \?, \?, 1\)`).
		WithArgs(stateTN, "Mayiladuthurai", 640, `["Mayavaram"]`).WillReturnResult(sqlmock.NewResult(700, 1))
	mock.ExpectQuery(byName).WithArgs(stateTN, "Coimbatore").WillReturnRows(idRows(coimbatore))
	mock.ExpectExec(update).WithArgs(stateTN, "Coimbatore", nil, `["Kovai"]`, coimbatore).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	res, err := ImportDistricts(context.Background(), conn, strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 1 || res.Updated != 2 || res.Skipped != 3 {
		t.Fatalf("result %+v", res)
	}
	skipped := map[int]bool{}
	for _, s := range res.SkippedLines {
		skipped[s.Line] = true
	}
	if !skipped[4] || !skipped[6] || !skipped[7] {
		t.Fatalf("skipped lines %+v", res.SkippedLines)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestImportDistricts_NoHeaderAndRollback(t *testing.T) {
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, code FROM states`).WillReturnRows(sqlmock.NewRows([]string{"id", "code"}).AddRow(stateTN, "TN"))
	mock.ExpectQuery(`SELECT id FROM districts WHERE lgd_code = \?`).WithArgs(600).WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()
	if _, err := ImportDistricts(context.Background(), conn, strings.NewReader("TN,Thanjavur,600\n")); err == nil {
		t.Fatal("want error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
