package search

import (
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/eligibility"
	"athlix/backend/internal/testutil"
)

const viewer = int64(42)

var now = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func decode(t *testing.T, body []byte) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return e
}

func setup(t *testing.T, cfg map[string]any) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return now }
	r, routes := testutil.UserEngine(testutil.VerifiedUser(viewer))
	New(d).Routes(routes)
	return d, mock, r
}

var personCols = []string{"id", "bio", "followers_count", "is_minor", "boosted"}

func summaryRows(photo any, ids ...int64) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "name", "role", "verification", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"})
	for _, id := range ids {
		rows.AddRow(id, "Asha K", "player", "verified", photo, "Pune", "Maharashtra", 3, "code", "Football", nil, nil, nil)
	}
	return rows
}

const summariesRe = `WHERE u.status <> 'deleted' AND u.id IN`

func count(n int) *sqlmock.Rows { return sqlmock.NewRows([]string{"n"}).AddRow(n) }

func TestPeople_FiltersAndExclusions(t *testing.T) {
	_, mock, r := setup(t, nil)
	like := `%as\_h\%%`
	mock.ExpectQuery(`SELECT t.id, t.bio[\s\S]*profile_boost[\s\S]*u.status = 'active' AND u.onboarding_step = 'done' AND u.id <> \?`+
		`[\s\S]*showInSearch[\s\S]*NOT EXISTS \(SELECT 1 FROM blocks bl[\s\S]*u.display_name LIKE \?[\s\S]*ur.role = \?`+
		`[\s\S]*p.sport_id = \?[\s\S]*p.city = \? AND \(u.is_minor = 0 OR[\s\S]*ORDER BY t.boosted DESC, t.id DESC LIMIT \?`).
		WithArgs(viewer, viewer, viewer, like, like, like, viewer, viewer, "player", 3, "Pune", viewer, viewer, 21).
		WillReturnRows(sqlmock.NewRows(personCols).AddRow(51, "Striker", 10, false, true).AddRow(50, nil, 0, false, false))
	mock.ExpectQuery(summariesRe).WithArgs(51, 50).WillReturnRows(summaryRows(nil, 50, 51))

	w := testutil.Request(r, http.MethodGet, "/v1/search/people?q=as_h%25&role=player&sportId=3&city=Pune", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	e := decode(t, w.Body.Bytes())
	var got []PersonResult
	_ = json.Unmarshal(e.Data, &got)
	if len(got) != 2 || got[0].ID != 51 || got[1].ID != 50 {
		t.Fatalf("order %s", e.Data)
	}
	if got[0].Bio == nil || *got[0].Bio != "Striker" || got[0].FollowersCount != 10 || got[0].City == nil {
		t.Fatalf("person %s", e.Data)
	}
	if e.Meta["nextCursor"] != nil && e.Meta["nextCursor"] != "" {
		t.Fatalf("unexpected cursor %v", e.Meta)
	}
}

func TestPeople_MinorFieldsHiddenUnlessMutual(t *testing.T) {
	d, mock, r := setup(t, nil)
	d.Users.SetPhotoSigner(d.Media)
	mock.ExpectQuery(`SELECT t.id, t.bio`).
		WillReturnRows(sqlmock.NewRows(personCols).AddRow(62, "Under 16 keeper", 4, true, false).AddRow(61, "Junior", 2, true, false))
	mock.ExpectQuery(summariesRe).WithArgs(62, 61).WillReturnRows(summaryRows(900, 61, 62))
	mock.ExpectQuery(`SELECT id, s3_key FROM media_uploads`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(900, "profile/x.jpg"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM follows`).WithArgs(viewer, 62, 62, viewer).WillReturnRows(count(2))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM follows`).WithArgs(viewer, 61, 61, viewer).WillReturnRows(count(1))

	w := testutil.Request(r, http.MethodGet, "/v1/search/people", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var got []PersonResult
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
	if len(got) != 2 {
		t.Fatalf("got %d", len(got))
	}
	mutual, other := got[0], got[1]
	if mutual.PhotoURL == nil || mutual.City == nil || mutual.Bio == nil {
		t.Fatalf("mutual follow should see minor fields: %+v", mutual)
	}
	if other.PhotoURL != nil || other.City != nil || other.Bio != nil {
		t.Fatalf("minor fields leaked: %+v", other)
	}
	if other.State == nil || other.DisplayName == "" {
		t.Fatalf("minor card should keep name and state: %+v", other)
	}
}

func TestPeople_MinorsNotSearchable(t *testing.T) {
	_, mock, r := setup(t, map[string]any{"minor.searchable": false})
	mock.ExpectQuery(`AND u.is_minor = 0\) t ORDER BY`).WillReturnRows(sqlmock.NewRows(personCols))

	w := testutil.Request(r, http.MethodGet, "/v1/search/people", nil, nil)
	if w.Code != http.StatusOK || string(decode(t, w.Body.Bytes()).Data) != `[]` {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

const benefitsRe = `SELECT b.benefit_key, b.value`

func benefits(keys ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"benefit_key", "value"})
	for _, k := range keys {
		rows.AddRow(k, []byte("true"))
	}
	return rows
}

func TestPeople_AdvancedFiltersNeedPlan(t *testing.T) {
	for _, qs := range []string{"ageMin=16", "ageMax=18", "achievementLevel=state"} {
		t.Run(qs, func(t *testing.T) {
			_, mock, r := setup(t, nil)
			mock.ExpectQuery(benefitsRe).WithArgs(viewer).WillReturnRows(benefits("profile_boost"))
			w := testutil.Request(r, http.MethodGet, "/v1/search/people?"+qs, nil, nil)
			e := decode(t, w.Body.Bytes())
			if w.Code != http.StatusForbidden || e.Error.Code != "PLAN_REQUIRED" || e.Error.Fields["benefit"] != "advanced_search_filters" {
				t.Fatalf("status %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestPeople_AdvancedFiltersWithPlan(t *testing.T) {
	_, mock, r := setup(t, nil)
	mock.ExpectQuery(benefitsRe).WithArgs(viewer).WillReturnRows(benefits("advanced_search_filters"))
	mock.ExpectQuery(`p.birth_year <= \?[\s\S]*p.birth_year >= \?[\s\S]*a.level = \?`).
		WithArgs(viewer, viewer, viewer, 2010, 2008, "state", 21).
		WillReturnRows(sqlmock.NewRows(personCols))

	w := testutil.Request(r, http.MethodGet, "/v1/search/people?ageMin=16&ageMax=18&achievementLevel=state", nil, nil)
	if w.Code != http.StatusOK || string(decode(t, w.Body.Bytes()).Data) != `[]` {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestPeople_Validation(t *testing.T) {
	cases := []struct {
		qs, field string
		plan      bool
	}{
		{"role=fan", "role", false},
		{"sportId=abc", "sportId", false},
		{"ageMin=old", "ageMin", false},
		{"ageMin=20&ageMax=10", "ageMin", true},
		{"ageMax=200", "ageMax", true},
		{"achievementLevel=galactic", "achievementLevel", true},
	}
	for _, tc := range cases {
		t.Run(tc.qs, func(t *testing.T) {
			_, mock, r := setup(t, nil)
			if tc.plan {
				mock.ExpectQuery(benefitsRe).WillReturnRows(benefits("advanced_search_filters"))
			}
			w := testutil.Request(r, http.MethodGet, "/v1/search/people?"+tc.qs, nil, nil)
			e := decode(t, w.Body.Bytes())
			if w.Code != http.StatusUnprocessableEntity || e.Error.Fields[tc.field] == "" {
				t.Fatalf("status %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestPeople_PageSizeAndCursor(t *testing.T) {
	_, mock, r := setup(t, map[string]any{"search.page_size": 2})
	mock.ExpectQuery(`ORDER BY t.boosted DESC, t.id DESC LIMIT \?`).
		WithArgs(viewer, viewer, viewer, 3).
		WillReturnRows(sqlmock.NewRows(personCols).AddRow(90, nil, 0, false, true).AddRow(80, nil, 0, false, true).AddRow(70, nil, 0, false, false))
	mock.ExpectQuery(summariesRe).WithArgs(90, 80).WillReturnRows(summaryRows(nil, 90, 80))

	w := testutil.Request(r, http.MethodGet, "/v1/search/people", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	cur, _ := decode(t, w.Body.Bytes()).Meta["nextCursor"].(string)
	raw, err := base64.RawURLEncoding.DecodeString(cur)
	if err != nil || string(raw) != `{"i":80,"t":"1"}` {
		t.Fatalf("cursor %q %s", cur, raw)
	}

	mock.ExpectQuery(`\) t WHERE \(t.boosted < \? OR \(t.boosted = \? AND t.id < \?\)\) ORDER BY`).
		WithArgs(viewer, viewer, viewer, 1, 1, 80, 3).
		WillReturnRows(sqlmock.NewRows(personCols).AddRow(70, nil, 0, false, false))
	mock.ExpectQuery(summariesRe).WithArgs(70).WillReturnRows(summaryRows(nil, 70))
	w = testutil.Request(r, http.MethodGet, "/v1/search/people?cursor="+cur, nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if e := decode(t, w.Body.Bytes()); e.Meta["nextCursor"] != nil && e.Meta["nextCursor"] != "" {
		t.Fatalf("last page cursor %v", e.Meta)
	}
}

var (
	eventCols = []string{"id", "title", "sid", "scode", "sname", "event_type", "participant_type", "competition_level",
		"gender_category", "age_min", "age_max", "venue_name", "city", "district", "state", "starts_at", "ends_at",
		"registration_deadline", "entry_fee_paise", "max_slots", "registrations_count", "banner_upload_id", "status",
		"featured_at", "organiser_id"}
	communityCols = []string{"id", "title", "description", "avatar_upload_id", "sid", "scode", "sname", "city", "state",
		"visibility", "join_policy", "member_count", "owner_id", "created_at", "cm_status", "cm_role"}
)

func TestGlobal_Shape(t *testing.T) {
	_, mock, r := setup(t, nil)
	like := "%ab%"
	mock.ExpectQuery(`SELECT t.id, t.bio`).WithArgs(viewer, viewer, viewer, like, like, like, viewer, viewer, 5).
		WillReturnRows(sqlmock.NewRows(personCols).AddRow(51, nil, 0, false, false))
	expectIdentities(mock)
	mock.ExpectQuery(`FROM events e[\s\S]*e.status = 'published'[\s\S]*NOT EXISTS \(SELECT 1 FROM blocks bl[\s\S]*AND \(e.eligibility_type = 'open'\) ORDER BY e.starts_at ASC, e.id ASC LIMIT \?`).
		WithArgs(like, like, like, viewer, viewer, 5).
		WillReturnRows(sqlmock.NewRows(eventCols).
			AddRow(7, "Abhi Cup", 3, "football", "Football", "tournament", "individual", "district", "open", nil, 18,
				"Shivaji Ground", "Pune", nil, "Maharashtra", now.Add(48*time.Hour), now.Add(72*time.Hour), now.Add(24*time.Hour),
				50000, 10, 12, 300, "published", now, 77).
			AddRow(8, "Abacus Run", 4, "athletics", "Athletics", "trial", "individual", "club", "male", 10, 14,
				"Track", "Pune", "Haveli", "Maharashtra", now.Add(96*time.Hour), now.Add(100*time.Hour), now.Add(90*time.Hour),
				0, 30, 5, nil, "published", nil, 78))
	mock.ExpectQuery(`FROM conversations c[\s\S]*c.type = 'community' AND c.visibility = 'public' AND c.status = 'active'`).
		WithArgs(viewer, like, like, viewer, viewer, 5).
		WillReturnRows(sqlmock.NewRows(communityCols).
			AddRow(20, "Pune Ballers", "Weekend games", 400, 3, "football", "Football", "Pune", "Maharashtra", "public", "open", 120, 77, now, "active", "admin").
			AddRow(21, "Abs club", nil, nil, nil, nil, nil, nil, nil, "public", "approval", 9, nil, now, "left", "member"))
	mock.ExpectQuery(summariesRe).WithArgs(51).WillReturnRows(summaryRows(nil, 51))
	mock.ExpectQuery(summariesRe).WithArgs(77, 78).WillReturnRows(summaryRows(nil, 77, 78))
	mock.ExpectQuery(`SELECT id, s3_key FROM media_uploads`).WithArgs(300, 400).
		WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(300, "event/b.jpg").AddRow(400, "community/a.jpg"))

	w := testutil.Request(r, http.MethodGet, "/v1/search?q=%20ab%20", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var got GlobalResult
	if err := json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.People) != 1 || got.People[0].ID != 51 {
		t.Fatalf("people %+v", got.People)
	}
	if len(got.Events) != 2 {
		t.Fatalf("events %+v", got.Events)
	}
	e0, e1 := got.Events[0], got.Events[1]
	if e0.SlotsLeft != 0 || !e0.IsFeatured || e0.Sport.Code != "football" || e0.BannerURL == nil || e0.Organiser == nil || e0.Organiser.ID != 77 {
		t.Fatalf("event 7 %+v", e0)
	}
	if e1.SlotsLeft != 25 || e1.IsFeatured || e1.BannerURL != nil || e1.District == nil || *e1.AgeMin != 10 {
		t.Fatalf("event 8 %+v", e1)
	}
	if e0.StartsAt != "2026-11-15T10:00:00Z" || e0.AgeMin != nil {
		t.Fatalf("event 7 times/ages %+v", e0)
	}
	if len(got.Communities) != 2 {
		t.Fatalf("communities %+v", got.Communities)
	}
	c0, c1 := got.Communities[0], got.Communities[1]
	if c0.MyStatus != "active" || c0.MyRole == nil || *c0.MyRole != "admin" || c0.AvatarURL == nil || c0.Owner == nil || c0.Sport == nil {
		t.Fatalf("community 20 %+v", c0)
	}
	if c1.MyStatus != "none" || c1.MyRole != nil || c1.Owner != nil || c1.Sport != nil || c1.Description != nil {
		t.Fatalf("community 21 %+v", c1)
	}
}

func TestGlobal_EmptyResultsAreArrays(t *testing.T) {
	_, mock, r := setup(t, nil)
	mock.ExpectQuery(`SELECT t.id, t.bio`).WillReturnRows(sqlmock.NewRows(personCols))
	expectIdentities(mock)
	mock.ExpectQuery(`FROM events e`).WillReturnRows(sqlmock.NewRows(eventCols))
	mock.ExpectQuery(`FROM conversations c`).WillReturnRows(sqlmock.NewRows(communityCols))

	w := testutil.Request(r, http.MethodGet, "/v1/search?q=zz", nil, nil)
	if w.Code != http.StatusOK || string(decode(t, w.Body.Bytes()).Data) != `{"people":[],"events":[],"communities":[]}` {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestGlobal_Validation(t *testing.T) {
	for _, q := range []string{"", "%20%20", "a"} {
		t.Run(q, func(t *testing.T) {
			_, _, r := setup(t, nil)
			w := testutil.Request(r, http.MethodGet, "/v1/search?q="+q, nil, nil)
			if w.Code != http.StatusUnprocessableEntity || decode(t, w.Body.Bytes()).Error.Fields["q"] == "" {
				t.Fatalf("status %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestLikePatternEscapesWildcards(t *testing.T) {
	if got := likePattern(`50%_off\`); got != `%50\%\_off\\%` {
		t.Fatalf("got %q", got)
	}
}

// ---- location eligibility (platform/eligibility FeedFilter on events) ----

var identityCols = []string{"id", "identity_type", "district_id", "state_id", "status", "expires_at"}

// expectIdentities is the FeedFilter load of the viewer's location identities (none by default).
func expectIdentities(mock sqlmock.Sqlmock, rows ...[]driver.Value) {
	rs := sqlmock.NewRows(identityCols)
	for _, r := range rows {
		rs.AddRow(r...)
	}
	mock.ExpectQuery(`FROM user_location_identities WHERE user_id = \?`).WithArgs(viewer).WillReturnRows(rs)
}

func globalWithIdentities(t *testing.T, cfg map[string]any, eventsRe string, eventArgs []driver.Value, ids ...[]driver.Value) {
	t.Helper()
	_, mock, r := setup(t, cfg)
	mock.ExpectQuery(`SELECT t.id, t.bio`).WillReturnRows(sqlmock.NewRows(personCols))
	if cfg == nil {
		expectIdentities(mock, ids...)
	}
	mock.ExpectQuery(eventsRe).WithArgs(eventArgs...).WillReturnRows(sqlmock.NewRows(eventCols))
	mock.ExpectQuery(`FROM conversations c`).WillReturnRows(sqlmock.NewRows(communityCols))
	if w := testutil.Request(r, http.MethodGet, "/v1/search?q=cup", nil, nil); w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestGlobal_EventsEligibility(t *testing.T) {
	like := "%cup%"
	base := []driver.Value{like, like, like, viewer, viewer}
	with := func(extra ...driver.Value) []driver.Value {
		return append(append(append([]driver.Value{}, base...), extra...), 5)
	}
	district := `\(\(e.eligibility_scope = 'district' AND e.eligibility_district_id = \?\) OR \(e.eligibility_scope = 'state' AND e.eligibility_state_id = \?\)\)`
	native := `\(e.eligibility_type IN \('native','either'\) AND \(`
	residence := `\(e.eligibility_type IN \('residence','either'\) AND \(`
	future := now.Add(30 * 24 * time.Hour)

	t.Run("no identity sees open events only", func(t *testing.T) {
		globalWithIdentities(t, nil, `AND \(e.eligibility_type = 'open'\) ORDER BY`, with())
	})
	t.Run("verified native", func(t *testing.T) {
		globalWithIdentities(t, nil, `AND \(e.eligibility_type = 'open' OR `+native+district+`\)\)\) ORDER BY`,
			with(eligibility.TestDistrictThanj, eligibility.TestStateTN),
			[]driver.Value{1, "native", eligibility.TestDistrictThanj, eligibility.TestStateTN, "verified", nil})
	})
	t.Run("verified residence", func(t *testing.T) {
		globalWithIdentities(t, nil, `AND \(e.eligibility_type = 'open' OR `+residence+district+`\)\)\) ORDER BY`,
			with(eligibility.TestDistrictCoimb, eligibility.TestStateTN),
			[]driver.Value{2, "residence", eligibility.TestDistrictCoimb, eligibility.TestStateTN, "verified", future})
	})
	t.Run("native and residence (either) both match", func(t *testing.T) {
		globalWithIdentities(t, nil, `AND \(e.eligibility_type = 'open' OR `+native+district+`\)\) OR `+residence+district+`\)\)\) ORDER BY`,
			with(eligibility.TestDistrictThanj, eligibility.TestStateTN, eligibility.TestDistrictErnaku, eligibility.TestStateKL),
			[]driver.Value{1, "native", eligibility.TestDistrictThanj, eligibility.TestStateTN, "verified", nil},
			[]driver.Value{2, "residence", eligibility.TestDistrictErnaku, eligibility.TestStateKL, "verified", future})
	})
	t.Run("pending proof only matches requireVerified=false", func(t *testing.T) {
		globalWithIdentities(t, nil, `AND \(e.eligibility_type = 'open' OR `+residence+`\(e.require_verified = 0 AND \(`+district+`\)\)\)\)\) ORDER BY`,
			with(eligibility.TestDistrictCoimb, eligibility.TestStateTN),
			[]driver.Value{2, "residence", eligibility.TestDistrictCoimb, eligibility.TestStateTN, "pending", nil})
	})
	t.Run("expired proof is ignored", func(t *testing.T) {
		globalWithIdentities(t, nil, `AND \(e.eligibility_type = 'open'\) ORDER BY`, with(),
			[]driver.Value{2, "residence", eligibility.TestDistrictCoimb, eligibility.TestStateTN, "verified", now.Add(-time.Hour)})
	})
	t.Run("switch off shows every event", func(t *testing.T) {
		globalWithIdentities(t, map[string]any{"location.enabled": false}, `AND 1=1 ORDER BY`, with())
	})
}
