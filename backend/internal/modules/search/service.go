package search

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/entitlements"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/userdir"
)

// Service runs read-only searches over other modules' tables.
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

// PageSize is the default people page size (search.page_size).
func (s *Service) PageSize(ctx context.Context) int {
	n := s.d.Settings.Int(ctx, "search.page_size", defaultPageSize)
	if n < 1 {
		return defaultPageSize
	}
	return n
}

// People runs GET /search/people. Advanced filters need the advanced_search_filters benefit.
func (s *Service) People(ctx context.Context, viewer int64, f PeopleFilter, p httpx.PageParams) ([]PersonResult, string, error) {
	if f.advanced() {
		ok, err := s.d.Entitlements.Has(ctx, viewer, entitlements.AdvancedSearchFilters)
		if err != nil {
			return nil, "", httpx.Internal().Wrap(err)
		}
		if !ok {
			e := httpx.Coded(http.StatusForbidden, httpx.CodePlanRequired, "Advanced filters need a premium plan.")
			e.Fields = map[string]string{"benefit": entitlements.AdvancedSearchFilters}
			return nil, "", e
		}
	}
	if err := validatePeople(&f); err != nil {
		return nil, "", err
	}
	rows, err := s.repo.people(ctx, peopleQuery{
		Viewer:        viewer,
		Filter:        f,
		MinorsAllowed: s.d.Settings.Bool(ctx, "minor.searchable", defaultMinors),
		Year:          s.d.Now().In(ist).Year(),
		Cursor:        p.Cursor,
		Limit:         p.Limit + 1,
	})
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		last := rows[len(rows)-1]
		b := "0"
		if last.Boosted {
			b = "1"
		}
		next = httpx.EncodeCursor(httpx.Cursor{ID: last.ID, Time: b})
	}
	out, err := s.toPeople(ctx, viewer, rows)
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	return out, next, nil
}

func validatePeople(f *PeopleFilter) error {
	v := map[string]string{}
	f.Q = strings.TrimSpace(f.Q)
	if utf8.RuneCountInString(f.Q) > maxQueryLen {
		v["q"] = "must be at most 100 characters"
	}
	if f.Role != "" && !roles[f.Role] {
		v["role"] = "must be one of player, coach, organiser"
	}
	for name, val := range map[string]*string{"state": &f.State, "district": &f.District, "city": &f.City} {
		*val = strings.TrimSpace(*val)
		if utf8.RuneCountInString(*val) > maxPlaceLen {
			v[name] = "must be at most 64 characters"
		}
	}
	if f.AgeMin != nil && (*f.AgeMin < 0 || *f.AgeMin > maxAge) {
		v["ageMin"] = "must be between 0 and 120"
	}
	if f.AgeMax != nil && (*f.AgeMax < 0 || *f.AgeMax > maxAge) {
		v["ageMax"] = "must be between 0 and 120"
	}
	if f.AgeMin != nil && f.AgeMax != nil && *f.AgeMin > *f.AgeMax && v["ageMin"] == "" && v["ageMax"] == "" {
		v["ageMin"] = "must not be greater than ageMax"
	}
	if f.AchievementLevel != "" && !achievementLevels[f.AchievementLevel] {
		v["achievementLevel"] = "is not a valid level"
	}
	if len(v) > 0 {
		return httpx.Validation(v)
	}
	return nil
}

// toPeople attaches summaries and strips a minor's photo, city and bio unless the viewer
// is a mutual follow (docs/security.md minors rule).
func (s *Service) toPeople(ctx context.Context, viewer int64, rows []personRow) ([]PersonResult, error) {
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	sums, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]PersonResult, 0, len(rows))
	for _, r := range rows {
		sum, ok := sums[r.ID]
		if !ok {
			continue
		}
		res := PersonResult{Summary: sum, Bio: r.Bio, FollowersCount: r.FollowersCount}
		if r.IsMinor {
			mutual, err := s.d.Users.MutualFollow(ctx, viewer, r.ID)
			if err != nil {
				return nil, err
			}
			if !mutual {
				res.PhotoURL, res.City, res.Bio = nil, nil, nil
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// Global runs GET /search: up to 5 people, events and communities matching q.
func (s *Service) Global(ctx context.Context, viewer int64, q string) (GlobalResult, error) {
	q = strings.TrimSpace(q)
	switch n := utf8.RuneCountInString(q); {
	case n == 0:
		return GlobalResult{}, httpx.Validation(map[string]string{"q": "is required"})
	case n < minQueryLen:
		return GlobalResult{}, httpx.Validation(map[string]string{"q": "must be at least 2 characters"})
	case n > maxQueryLen:
		return GlobalResult{}, httpx.Validation(map[string]string{"q": "must be at most 100 characters"})
	}
	people, err := s.repo.people(ctx, peopleQuery{
		Viewer:        viewer,
		Filter:        PeopleFilter{Q: q},
		MinorsAllowed: s.d.Settings.Bool(ctx, "minor.searchable", defaultMinors),
		Limit:         globalMax,
	})
	if err != nil {
		return GlobalResult{}, httpx.Internal().Wrap(err)
	}
	elCond, elArgs, err := s.d.Eligibility.FeedFilter(ctx, "e", viewer, s.d.Now())
	if err != nil {
		return GlobalResult{}, httpx.Internal().Wrap(err)
	}
	evs, err := s.repo.events(ctx, viewer, q, elCond, elArgs, globalMax)
	if err != nil {
		return GlobalResult{}, httpx.Internal().Wrap(err)
	}
	comms, err := s.repo.communities(ctx, viewer, q, globalMax)
	if err != nil {
		return GlobalResult{}, httpx.Internal().Wrap(err)
	}
	return s.assemble(ctx, viewer, people, evs, comms)
}

// assemble loads every user card and media URL in one call each.
func (s *Service) assemble(ctx context.Context, viewer int64, people []personRow, evs []eventRow, comms []communityRow) (GlobalResult, error) {
	res := GlobalResult{People: []userdir.Summary{}, Events: []EventSummary{}, Communities: []Community{}}
	persons, err := s.toPeople(ctx, viewer, people)
	if err != nil {
		return res, httpx.Internal().Wrap(err)
	}
	for _, p := range persons {
		res.People = append(res.People, p.Summary)
	}

	var userIDs, mediaIDs []int64
	for _, e := range evs {
		userIDs = append(userIDs, e.OrganiserID)
		if e.BannerID != nil {
			mediaIDs = append(mediaIDs, *e.BannerID)
		}
	}
	for _, c := range comms {
		if c.OwnerID != nil {
			userIDs = append(userIDs, *c.OwnerID)
		}
		if c.AvatarID != nil {
			mediaIDs = append(mediaIDs, *c.AvatarID)
		}
	}
	sums, err := s.d.Users.Summaries(ctx, dedupe(userIDs))
	if err != nil {
		return res, httpx.Internal().Wrap(err)
	}
	urls := s.d.Media.ViewURLs(ctx, dedupe(mediaIDs))

	for _, e := range evs {
		ev := e.EventSummary
		if sum, ok := sums[e.OrganiserID]; ok {
			ev.Organiser = &sum
		}
		if e.BannerID != nil {
			ev.BannerURL = urlPtr(urls, *e.BannerID)
		}
		res.Events = append(res.Events, ev)
	}
	for _, c := range comms {
		cm := c.Community
		if c.OwnerID != nil {
			if sum, ok := sums[*c.OwnerID]; ok {
				cm.Owner = &sum
			}
		}
		if c.AvatarID != nil {
			cm.AvatarURL = urlPtr(urls, *c.AvatarID)
		}
		res.Communities = append(res.Communities, cm)
	}
	return res, nil
}

func urlPtr(urls map[int64]string, id int64) *string {
	if u, ok := urls[id]; ok && u != "" {
		return &u
	}
	return nil
}

func dedupe(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
