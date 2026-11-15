package event

import (
	"context"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
)

// Summaries returns EventSummary views of the given events as seen by viewer uid, keyed by
// event id (missing ids are absent). It is the read the registration module (same owner)
// uses for GET /me/registrations, so both modules render EventSummary the same way.
func Summaries(ctx context.Context, d *app.Deps, uid int64, ids []int64) (map[int64]Summary, error) {
	out := map[int64]Summary{}
	if len(ids) == 0 {
		return out, nil
	}
	s := newService(d)
	rs, err := s.r.queryRecs(ctx, false, `SELECT `+eventCols+eventFrom+` WHERE e.id IN (`+db.Placeholders(len(ids))+`)`, db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	sums, err := s.summaries(ctx, s.newViewer(ctx, uid), rs, false)
	if err != nil {
		return nil, err
	}
	for _, sm := range sums {
		out[sm.ID] = sm
	}
	return out, nil
}
