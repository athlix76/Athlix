package sport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/testutil"
)

var (
	sportCols = []string{"id", "code", "name", "team", "active", "sort"}
	posCols   = []string{"id", "sport_id", "code", "name", "applies", "active", "sort"}
	tplCols   = []string{"id", "sport_id", "position_id", "name", "version", "fields", "active"}
)

func TestPublicSports(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	r, rt := testutil.UserEngine(testutil.VerifiedUser(1))
	New(d).Routes(rt)
	mock.ExpectQuery(`FROM sports WHERE active = 1 ORDER BY sort_order, id`).WillReturnRows(sqlmock.NewRows(sportCols).AddRow(1, "cricket", "Cricket", true, true, 0))
	mock.ExpectQuery(`FROM sport_positions p JOIN sports s ON s.id = p.sport_id WHERE 1 = 1 AND p.active = 1`).WillReturnRows(sqlmock.NewRows(posCols).AddRow(5, 1, "batter", "Batter", "player", true, 0).AddRow(6, 2, "x", "X", "player", true, 0))
	mock.ExpectQuery(`FROM stat_templates WHERE active = 1`).WillReturnRows(sqlmock.NewRows(tplCols).AddRow(3, 1, nil, "Season", 2, `[{"key":"runs","label":"Runs","type":"int","unit":null}]`, true))
	w := testutil.Request(r, "GET", "/v1/sports", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var b struct{ Data []Sport }
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if len(b.Data) != 1 || len(b.Data[0].Positions) != 1 || b.Data[0].StatTemplates[0].Fields[0].Key != "runs" || b.Data[0].StatTemplates[0].PositionID != nil {
		t.Fatalf("%+v", b.Data)
	}
	if strings.Contains(w.Body.String(), "sortOrder") || strings.Contains(w.Body.String(), `"active"`) {
		t.Fatal("public catalogue must not show admin flags")
	}
}

func TestAdminCatalogueIncludesInactive(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	mock.ExpectQuery(`FROM sports ORDER BY sort_order, id`).WillReturnRows(sqlmock.NewRows(sportCols).AddRow(1, "cricket", "Cricket", true, false, 4))
	mock.ExpectQuery(`FROM sport_positions p JOIN sports s ON s.id = p.sport_id WHERE 1 = 1 ORDER BY`).WillReturnRows(sqlmock.NewRows(posCols).AddRow(5, 1, "batter", "Batter", "both", false, 1))
	mock.ExpectQuery(`FROM stat_templates ORDER BY`).WillReturnRows(sqlmock.NewRows(tplCols).AddRow(3, 1, 5, "Batting", 1, `bad`, false))
	out, err := Catalogue(context.Background(), d.DB, true)
	if err != nil || len(out) != 1 || *out[0].Active || *out[0].SortOrder != 4 || *out[0].Positions[0].Active || *out[0].StatTemplates[0].PositionID != 5 || len(out[0].StatTemplates[0].Fields) != 0 {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestEmptyCatalogue(t *testing.T) {
	d, mock := testutil.Deps(t, nil)
	mock.ExpectQuery(`FROM sports`).WillReturnRows(sqlmock.NewRows(sportCols))
	out, err := Catalogue(context.Background(), d.DB, false)
	if err != nil || out == nil || len(out) != 0 {
		t.Fatal(out, err)
	}
}
