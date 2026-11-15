package sport

import (
	"context"

	"athlix/backend/internal/platform/db"
)

// Catalogue loads every sport with its positions and stat templates, ordered by sort_order
// then id. With includeInactive=false only active rows are returned (the public catalogue);
// with true every row is returned with its active / sortOrder flags (admin editors).
func Catalogue(ctx context.Context, q db.Querier, includeInactive bool) ([]Sport, error) {
	active := ""
	if !includeInactive {
		active = " WHERE active = 1"
	}
	rows, err := q.QueryContext(ctx, `SELECT id, code, name, is_team_sport, active, sort_order FROM sports`+active+` ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	out := []Sport{}
	index := map[int64]int{}
	for rows.Next() {
		var s Sport
		var act bool
		var sort int
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.IsTeamSport, &act, &sort); err != nil {
			rows.Close()
			return nil, err
		}
		s.Positions, s.StatTemplates = []Position{}, []StatTemplate{}
		if includeInactive {
			s.Active, s.SortOrder = &act, &sort
		}
		index[s.ID] = len(out)
		out = append(out, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	activeChild := ""
	if !includeInactive {
		activeChild = " AND p.active = 1"
	}
	prow, err := q.QueryContext(ctx, `SELECT p.id, p.sport_id, p.code, p.name, p.applies_to, p.active, p.sort_order FROM sport_positions p JOIN sports s ON s.id = p.sport_id WHERE 1 = 1`+activeChild+` ORDER BY p.sport_id, p.sort_order, p.id`)
	if err != nil {
		return nil, err
	}
	for prow.Next() {
		var p Position
		var sportID int64
		var act bool
		var sort int
		if err := prow.Scan(&p.ID, &sportID, &p.Code, &p.Name, &p.AppliesTo, &act, &sort); err != nil {
			prow.Close()
			return nil, err
		}
		if includeInactive {
			p.Active, p.SortOrder = &act, &sort
		}
		if i, ok := index[sportID]; ok {
			out[i].Positions = append(out[i].Positions, p)
		}
	}
	prow.Close()
	if err := prow.Err(); err != nil {
		return nil, err
	}

	activeTpl := ""
	if !includeInactive {
		activeTpl = " WHERE active = 1"
	}
	trow, err := q.QueryContext(ctx, `SELECT id, sport_id, position_id, name, version, fields, active FROM stat_templates`+activeTpl+` ORDER BY sport_id, id`)
	if err != nil {
		return nil, err
	}
	defer trow.Close()
	for trow.Next() {
		var t StatTemplate
		var sportID int64
		var pos *int64
		var fields []byte
		var act bool
		if err := trow.Scan(&t.ID, &sportID, &pos, &t.Name, &t.Version, &fields, &act); err != nil {
			return nil, err
		}
		t.PositionID = pos
		t.Fields = ParseFields(fields)
		if includeInactive {
			t.Active = &act
		}
		if i, ok := index[sportID]; ok {
			out[i].StatTemplates = append(out[i].StatTemplates, t)
		}
	}
	return out, trow.Err()
}
