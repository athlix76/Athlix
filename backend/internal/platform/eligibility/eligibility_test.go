package eligibility

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/settings"
)

var now = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

func ptr(t time.Time) *time.Time { return &t }

func nativeThanjavur(status string) Identity {
	return Identity{ID: 1, Type: TypeNative, DistrictID: TestDistrictThanj, StateID: TestStateTN, Status: status}
}

func residenceCoimbatore(status string, exp time.Time) Identity {
	return Identity{ID: 2, Type: TypeResidence, DistrictID: TestDistrictCoimb, StateID: TestStateTN, Status: status, ExpiresAt: ptr(exp)}
}

var (
	ruleNativeThanj = Rule{Type: TypeNative, Scope: ScopeDistrict, DistrictID: TestDistrictThanj, RequireVerified: true}
	ruleResCoimb    = Rule{Type: TypeResidence, Scope: ScopeDistrict, DistrictID: TestDistrictCoimb, RequireVerified: true}
)

// The five cases from the spec, plus either / state / require_verified=false.
func TestEvaluate(t *testing.T) {
	future, past := now.AddDate(0, 6, 0), now.AddDate(0, 0, -1)
	cases := []struct {
		name     string
		rule     Rule
		ids      []Identity
		eligible bool
		via      string
		reason   string
		action   string
		pending  bool
	}{
		{"open event, no identities", Rule{Type: TypeOpen}, nil, true, TypeOpen, "", ActionNone, false},
		{"eligible native user", ruleNativeThanj, []Identity{nativeThanjavur("verified")}, true, TypeNative, "", ActionNone, false},
		{"eligible residence user", ruleResCoimb, []Identity{residenceCoimbatore("verified", future)}, true, TypeResidence, "", ActionNone, false},
		{"ineligible: native of another district", ruleResCoimb, []Identity{nativeThanjavur("verified")}, false, "", ReasonIdentityMissing, ActionVerifyResidence, false},
		{"ineligible: wrong district", ruleNativeThanj, []Identity{{ID: 3, Type: TypeNative, DistrictID: TestDistrictCoimb, StateID: TestStateTN, Status: "verified"}}, false, "", ReasonWrongLocation, ActionVerifyNative, false},
		{"expired proof (past expires_at, job not run yet)", ruleResCoimb, []Identity{residenceCoimbatore("verified", past)}, false, "", ReasonIdentityExpired, ActionRenewResidence, false},
		{"expired proof (status expired)", ruleResCoimb, []Identity{residenceCoimbatore("expired", past)}, false, "", ReasonIdentityExpired, ActionRenewResidence, false},
		{"pending proof, event needs verified", ruleResCoimb, []Identity{residenceCoimbatore("pending", future)}, false, "", ReasonIdentityPending, ActionWaitForReview, false},
		{"pending proof, event allows pending", Rule{Type: TypeResidence, Scope: ScopeDistrict, DistrictID: TestDistrictCoimb}, []Identity{residenceCoimbatore("pending", future)}, true, TypeResidence, "", ActionWaitForReview, true},
		{"either: residence qualifies", Rule{Type: TypeEither, Scope: ScopeDistrict, DistrictID: TestDistrictCoimb, RequireVerified: true}, []Identity{nativeThanjavur("verified"), residenceCoimbatore("verified", future)}, true, TypeResidence, "", ActionNone, false},
		{"either: native preferred when both match", Rule{Type: TypeEither, Scope: ScopeState, StateID: TestStateTN, RequireVerified: true}, []Identity{residenceCoimbatore("verified", future), nativeThanjavur("verified")}, true, TypeNative, "", ActionNone, false},
		{"state scope: any district of the state", Rule{Type: TypeNative, Scope: ScopeState, StateID: TestStateTN, RequireVerified: true}, []Identity{nativeThanjavur("verified")}, true, TypeNative, "", ActionNone, false},
		{"state scope: other state", Rule{Type: TypeNative, Scope: ScopeState, StateID: TestStateKL, RequireVerified: true}, []Identity{nativeThanjavur("verified")}, false, "", ReasonWrongLocation, ActionVerifyNative, false},
		{"verified beats pending for the same event", Rule{Type: TypeEither, Scope: ScopeState, StateID: TestStateTN}, []Identity{residenceCoimbatore("pending", future), nativeThanjavur("verified")}, true, TypeNative, "", ActionNone, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Evaluate(c.rule, c.ids, now)
			if got.Eligible != c.eligible || got.Via != c.via || got.Reason != c.reason || got.Action != c.action || got.Pending != c.pending {
				t.Fatalf("got %+v", got)
			}
			if got.Eligible && got.Via != TypeOpen && got.IdentityID == 0 {
				t.Fatalf("qualifying identity missing: %+v", got)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	good := []Rule{
		{Type: TypeOpen},
		ruleNativeThanj,
		{Type: TypeEither, Scope: ScopeState, StateID: TestStateTN},
	}
	for _, r := range good {
		if f := r.Validate(); len(f) != 0 {
			t.Fatalf("%+v: %v", r, f)
		}
	}
	bad := map[string]Rule{
		"eligibility.type":       {Type: "city"},
		"eligibility.scope":      {Type: TypeNative},
		"eligibility.districtId": {Type: TypeNative, Scope: ScopeDistrict},
		"eligibility.stateId":    {Type: TypeResidence, Scope: ScopeState},
	}
	for field, r := range bad {
		if _, ok := r.Validate()[field]; !ok {
			t.Fatalf("%+v: want error on %s, got %v", r, field, r.Validate())
		}
	}
	if f := (Rule{Type: TypeOpen, Scope: ScopeDistrict}).Validate(); len(f) == 0 {
		t.Fatal("open with a scope must fail")
	}
}

func TestFilter(t *testing.T) {
	cond, args := Filter("e", nil, now)
	if cond != "(e.eligibility_type = 'open')" || len(args) != 0 {
		t.Fatalf("no identities: %q %v", cond, args)
	}
	ids := []Identity{nativeThanjavur("verified"), residenceCoimbatore("pending", now.AddDate(0, 1, 0)),
		{ID: 9, Type: TypeResidence, DistrictID: TestDistrictErnaku, StateID: TestStateKL, Status: "verified", ExpiresAt: ptr(now.Add(-time.Hour))}}
	cond, args = Filter("e", ids, now)
	if strings.Count(cond, "?") != len(args) {
		t.Fatalf("placeholders %d != args %d", strings.Count(cond, "?"), len(args))
	}
	if len(args) != 4 { // expired Ernakulam residence is skipped
		t.Fatalf("args %v", args)
	}
	if !strings.Contains(cond, "e.require_verified = 0") || !strings.Contains(cond, "IN ('native','either')") {
		t.Fatalf("cond %q", cond)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("bad alias must panic")
		}
	}()
	Filter("e; DROP", nil, now)
}

func TestDescribeAndTag(t *testing.T) {
	if got := Describe(ruleNativeThanj, "Thanjavur"); got != "Only players with a verified Thanjavur native identity can register." {
		t.Fatal(got)
	}
	if got := Describe(Rule{Type: TypeEither, Scope: ScopeState, StateID: 23}, "Tamil Nadu"); got != "Only players with a verified or in-review Tamil Nadu (any district) native or residence identity can register." {
		t.Fatal(got)
	}
	if Describe(Rule{Type: TypeOpen}, "") != "Open to all players." {
		t.Fatal("open")
	}
	if got := Tag(Result{Eligible: true, Via: TypeNative}, "Thanjavur"); got != "Eligible via Native: Thanjavur" {
		t.Fatal(got)
	}
	if got := Tag(Result{Eligible: true, Via: TypeResidence, Pending: true}, "Coimbatore"); got != "Eligible via Residence: Coimbatore (in review)" {
		t.Fatal(got)
	}
	if Tag(Result{Eligible: true, Via: TypeOpen}, "") != "" || Tag(Result{}, "x") != "" {
		t.Fatal("open / ineligible must have no tag")
	}
}

func TestMatchDistrict(t *testing.T) {
	p := TestPlaces()
	ctx := context.Background()
	for _, in := range []string{"Thanjavur", "TANJORE", " thanjavur. "} {
		d, ok := p.MatchDistrict(ctx, "Tamil Nadu", in)
		if !ok || d.ID != TestDistrictThanj {
			t.Fatalf("%q: %v %v", in, d, ok)
		}
	}
	if _, ok := p.MatchDistrict(ctx, "Kerala", "Thanjavur"); ok {
		t.Fatal("state must match")
	}
	if _, ok := p.MatchDistrict(ctx, "Tamil Nadu", ""); ok {
		t.Fatal("empty district")
	}
	if st, ok := p.State(ctx, TestStateTN); !ok || st.Code != "TN" {
		t.Fatal("state lookup")
	}
	if p.DistrictName(ctx, TestDistrictCoimb) != "Coimbatore" || p.StateName(ctx, TestStateTN) != "Tamil Nadu" {
		t.Fatal("names")
	}
}

func checker(t *testing.T, cfg map[string]any) (*Checker, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewWithPlaces(conn, settings.NewStatic(cfg), TestPlaces()), mock, conn
}

func TestCheckLoadsAndEvaluates(t *testing.T) {
	c, mock, _ := checker(t, nil)
	mock.ExpectQuery("FROM user_location_identities WHERE user_id = \\?").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "identity_type", "district_id", "state_id", "status", "expires_at"}).
			AddRow(7, "native", TestDistrictThanj, TestStateTN, "verified", nil))
	res, err := c.Check(context.Background(), nil, 42, ruleNativeThanj, now)
	if err != nil || !res.Eligible || res.IdentityID != 7 || res.DistrictID != TestDistrictThanj {
		t.Fatalf("%+v %v", res, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFeatureSwitchOff(t *testing.T) {
	c, mock, _ := checker(t, map[string]any{"location.enabled": false})
	res, err := c.Check(context.Background(), nil, 42, ruleNativeThanj, now)
	if err != nil || !res.Eligible || res.Via != TypeOpen {
		t.Fatalf("switch off must make every event open: %+v", res)
	}
	cond, args, err := c.FeedFilter(context.Background(), "e", 42, now)
	if err != nil || cond != "1=1" || args != nil {
		t.Fatalf("%q %v %v", cond, args, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil { // no DB call at all
		t.Fatal(err)
	}
}
