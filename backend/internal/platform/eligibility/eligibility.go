// Package eligibility decides whether a user may join an event based on location identities
// (native = from official ID, residence = where they live now). It is the ONE implementation
// of the rule: the event feed (SQL filter), event detail, registration, waitlist promotion and
// the registration re-check on identity changes all call it. Never re-implement it in a module.
//
// Rules (docs/api.md#location-identity, docs/architecture.md#location-eligibility):
//   - type open: everyone qualifies.
//   - type native / residence / either: the user needs an identity of that type (either: any)
//     whose district matches (scope district) or whose state matches (scope state).
//   - Only `verified` identities that have not passed expires_at count; `pending` ones count
//     too when the event has require_verified = false.
//   - Setting location.enabled = false makes every event open (feature switch).
//
// The package only READS user_location_identities, states and districts (owned by the
// locationidentity module).
package eligibility

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/settings"
)

// Event eligibility types.
const (
	TypeOpen      = "open"
	TypeNative    = "native"
	TypeResidence = "residence"
	TypeEither    = "either"

	ScopeDistrict = "district"
	ScopeState    = "state"
)

// Reasons an ineligible result carries (closed list, the app switches on them).
const (
	ReasonIdentityMissing = "identity_missing" // no identity of the required type
	ReasonIdentityPending = "identity_pending" // matching identity is still in review and the event needs verified
	ReasonIdentityExpired = "identity_expired" // matching residence identity expired
	ReasonWrongLocation   = "wrong_location"   // has the identity type, but for another district/state
)

// Actions the app offers next to an ineligible event ("Verify now").
const (
	ActionNone            = "none"
	ActionVerifyNative    = "verify_native"
	ActionVerifyResidence = "verify_residence"
	ActionRenewResidence  = "renew_residence"
	ActionWaitForReview   = "wait_for_review"
)

// Rule is the eligibility part of an events row.
type Rule struct {
	Type            string // open | native | residence | either
	Scope           string // district | state ("" for open)
	DistrictID      int64
	StateID         int64
	RequireVerified bool
}

// Identity is the subset of a user_location_identities row the rule needs.
type Identity struct {
	ID         int64
	Type       string // native | residence
	DistrictID int64
	StateID    int64
	Status     string // pending | verified | expired (others are never loaded)
	ExpiresAt  *time.Time
}

// Result is the decision for one user and one event.
type Result struct {
	Eligible   bool
	Via        string // open | native | residence ("" when not eligible)
	IdentityID int64  // the identity that qualified (0 for open)
	DistrictID int64  // district of that identity (0 for open)
	Pending    bool   // qualified with a pending identity (event allows pending)
	Reason     string // "" when eligible, else Reason*
	Action     string // Action*
}

// Validate checks a rule as entered by an organiser. It does not check that the ids exist.
func (r Rule) Validate() map[string]string {
	f := map[string]string{}
	switch r.Type {
	case TypeOpen:
		if r.Scope != "" || r.DistrictID != 0 || r.StateID != 0 {
			f["eligibility.scope"] = "must be empty for open events"
		}
	case TypeNative, TypeResidence, TypeEither:
		switch r.Scope {
		case ScopeDistrict:
			if r.DistrictID <= 0 {
				f["eligibility.districtId"] = "is required"
			}
		case ScopeState:
			if r.StateID <= 0 {
				f["eligibility.stateId"] = "is required"
			}
			if r.DistrictID != 0 {
				f["eligibility.districtId"] = "must be empty for a state rule"
			}
		default:
			f["eligibility.scope"] = "must be district or state"
		}
	default:
		f["eligibility.type"] = "must be open, native, residence or either"
	}
	return f
}

func (r Rule) types() []string {
	switch r.Type {
	case TypeNative:
		return []string{TypeNative}
	case TypeResidence:
		return []string{TypeResidence}
	case TypeEither:
		return []string{TypeNative, TypeResidence}
	}
	return nil
}

func (r Rule) matches(id Identity) bool {
	if r.Scope == ScopeState {
		return id.StateID == r.StateID
	}
	return id.DistrictID == r.DistrictID
}

func usable(id Identity, now time.Time) (verified, pending, expired bool) {
	switch id.Status {
	case "verified":
		if id.ExpiresAt != nil && !id.ExpiresAt.After(now) {
			return false, false, true
		}
		return true, false, false
	case "pending":
		return false, true, false
	case "expired":
		return false, false, true
	}
	return false, false, false
}

// Evaluate applies the rule to the user's identities. Pure function: easy to test.
func Evaluate(r Rule, ids []Identity, now time.Time) Result {
	if r.Type == TypeOpen || r.Type == "" {
		return Result{Eligible: true, Via: TypeOpen, Action: ActionNone}
	}
	types := r.types()
	var best *Result
	hasType, pendingMatch, expiredMatch := false, false, false
	for _, t := range types { // native first: it never expires, so it is the safer qualifier
		for _, id := range ids {
			if id.Type != t {
				continue
			}
			v, p, e := usable(id, now)
			if v || p || e {
				hasType = true
			}
			if !r.matches(id) {
				continue
			}
			switch {
			case v:
				return Result{Eligible: true, Via: t, IdentityID: id.ID, DistrictID: id.DistrictID, Action: ActionNone}
			case p && !r.RequireVerified:
				if best == nil {
					best = &Result{Eligible: true, Via: t, IdentityID: id.ID, DistrictID: id.DistrictID, Pending: true, Action: ActionWaitForReview}
				}
			case p:
				pendingMatch = true
			case e:
				expiredMatch = true
			}
		}
	}
	if best != nil {
		return *best
	}
	res := Result{Eligible: false}
	switch {
	case pendingMatch:
		res.Reason, res.Action = ReasonIdentityPending, ActionWaitForReview
	case expiredMatch:
		res.Reason, res.Action = ReasonIdentityExpired, ActionRenewResidence
	case hasType:
		res.Reason = ReasonWrongLocation
		res.Action = verifyAction(r)
	default:
		res.Reason = ReasonIdentityMissing
		res.Action = verifyAction(r)
	}
	return res
}

func verifyAction(r Rule) string {
	if r.Type == TypeResidence {
		return ActionVerifyResidence
	}
	return ActionVerifyNative // native or either: native is the permanent one
}

var aliasRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Filter returns a SQL condition (with bind args) that is true for events with alias `e`
// the identities qualify for (open events included). Use in feed / search / featured queries:
//
//	cond, args := eligibility.Filter("e", ids, now)
//	q := "SELECT ... FROM events e WHERE e.status = 'published' AND " + cond
//
// alias is code-provided (never user input); anything but a plain identifier panics.
func Filter(alias string, ids []Identity, now time.Time) (string, []any) {
	if !aliasRe.MatchString(alias) {
		panic("eligibility.Filter: bad alias")
	}
	a := alias + "."
	parts := []string{a + "eligibility_type = 'open'"}
	var args []any
	for _, t := range []string{TypeNative, TypeResidence} {
		var verified, pending []string
		var vArgs, pArgs []any
		for _, id := range ids {
			if id.Type != t {
				continue
			}
			v, p, _ := usable(id, now)
			if !v && !p {
				continue
			}
			m := "((" + a + "eligibility_scope = 'district' AND " + a + "eligibility_district_id = ?) OR (" +
				a + "eligibility_scope = 'state' AND " + a + "eligibility_state_id = ?))"
			if v {
				verified = append(verified, m)
				vArgs = append(vArgs, id.DistrictID, id.StateID)
			} else {
				pending = append(pending, m)
				pArgs = append(pArgs, id.DistrictID, id.StateID)
			}
		}
		if len(verified) == 0 && len(pending) == 0 {
			continue
		}
		var match []string
		if len(verified) > 0 {
			match = append(match, strings.Join(verified, " OR "))
			args = append(args, vArgs...)
		}
		if len(pending) > 0 {
			match = append(match, "("+a+"require_verified = 0 AND ("+strings.Join(pending, " OR ")+"))")
			args = append(args, pArgs...)
		}
		parts = append(parts, "("+a+"eligibility_type IN ('"+t+"','either') AND ("+strings.Join(match, " OR ")+"))")
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

// Describe is the one-line summary shown to organisers (preview) and players, e.g.
// "Only players with a verified Thanjavur native identity can register."
// place is the district or state name for the rule's scope.
func Describe(r Rule, place string) string {
	if r.Type == TypeOpen || r.Type == "" {
		return "Open to all players."
	}
	status := "verified"
	if !r.RequireVerified {
		status = "verified or in-review"
	}
	where := place
	if r.Scope == ScopeState {
		where = place + " (any district)"
	}
	kind := r.Type
	if r.Type == TypeEither {
		kind = "native or residence"
	}
	return fmt.Sprintf("Only players with a %s %s %s identity can register.", status, where, kind)
}

// Tag is the label on an event card, e.g. "Eligible via Native: Thanjavur". Empty for open
// events and ineligible results.
func Tag(res Result, place string) string {
	if !res.Eligible || res.Via == TypeOpen {
		return ""
	}
	kind := "Native"
	if res.Via == TypeResidence {
		kind = "Residence"
	}
	t := "Eligible via " + kind + ": " + place
	if res.Pending {
		t += " (in review)"
	}
	return t
}

// Checker loads identities and place names. Build once (app.Deps.Eligibility).
type Checker struct {
	db       *sql.DB
	settings *settings.Store
	places   *Places
}

// New builds a Checker.
func New(d *sql.DB, st *settings.Store) *Checker {
	return &Checker{db: d, settings: st, places: NewPlaces(d, 10*time.Minute)}
}

// Enabled reports the location.enabled switch. When false, callers treat every event as open.
func (c *Checker) Enabled(ctx context.Context) bool {
	return c.settings.Bool(ctx, "location.enabled", true)
}

// Places returns the cached state/district names.
func (c *Checker) Places() *Places { return c.places }

// Load returns the identities that can matter for eligibility (pending, verified, expired).
// q may be a transaction (registration re-check) or nil for the pool.
func (c *Checker) Load(ctx context.Context, q db.Querier, userID int64) ([]Identity, error) {
	if q == nil {
		q = c.db
	}
	rows, err := q.QueryContext(ctx, `SELECT id, identity_type, district_id, state_id, status, expires_at
		FROM user_location_identities WHERE user_id = ? AND status IN ('pending','verified','expired')
		ORDER BY id DESC LIMIT 20`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Identity
	for rows.Next() {
		var id Identity
		var exp sql.NullTime
		if err := rows.Scan(&id.ID, &id.Type, &id.DistrictID, &id.StateID, &id.Status, &exp); err != nil {
			return nil, err
		}
		if exp.Valid {
			t := exp.Time
			id.ExpiresAt = &t
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Check loads the user's identities and evaluates one rule (feature switch applied).
func (c *Checker) Check(ctx context.Context, q db.Querier, userID int64, r Rule, now time.Time) (Result, error) {
	if !c.Enabled(ctx) {
		return Evaluate(Rule{Type: TypeOpen}, nil, now), nil
	}
	ids, err := c.Load(ctx, q, userID)
	if err != nil {
		return Result{}, err
	}
	return Evaluate(r, ids, now), nil
}

// FeedFilter is Filter for one user with the feature switch applied ("1=1" when disabled).
func (c *Checker) FeedFilter(ctx context.Context, alias string, userID int64, now time.Time) (string, []any, error) {
	if !c.Enabled(ctx) {
		return "1=1", nil, nil
	}
	ids, err := c.Load(ctx, nil, userID)
	if err != nil {
		return "", nil, err
	}
	cond, args := Filter(alias, ids, now)
	return cond, args, nil
}

// PlaceFor returns the name used in Describe for a rule (district or state name).
func (c *Checker) PlaceFor(ctx context.Context, r Rule) string {
	if r.Scope == ScopeState {
		return c.places.StateName(ctx, r.StateID)
	}
	return c.places.DistrictName(ctx, r.DistrictID)
}

// Places caches the states and districts tables (small, rarely changed).
type Places struct {
	db  *sql.DB
	ttl time.Duration

	mu        sync.RWMutex
	loadedAt  time.Time
	states    map[int64]State
	districts map[int64]District

	// Cross-instance invalidation (places_version.go). All zero when no Redis is attached.
	vs          VersionStore
	every       time.Duration
	lastCheck   atomic.Int64 // unix nanos of the last version read
	version     int64        // version the loaded data belongs to
	haveVersion bool
}

// State is a states row.
type State struct {
	ID   int64
	Code string
	Name string
	IsUT bool
}

// District is a districts row.
type District struct {
	ID      int64
	StateID int64
	Name    string
	Aliases []string
	Active  bool
}

// NewPlaces builds the cache. NewStaticPlaces is for tests.
func NewPlaces(d *sql.DB, ttl time.Duration) *Places { return &Places{db: d, ttl: ttl} }

// NewStaticPlaces returns a cache that never hits the DB.
func NewStaticPlaces(states []State, districts []District) *Places {
	p := &Places{ttl: 1<<62 - 1, loadedAt: time.Now(), states: map[int64]State{}, districts: map[int64]District{}}
	for _, s := range states {
		p.states[s.ID] = s
	}
	for _, d := range districts {
		p.districts[d.ID] = d
	}
	return p
}

// Invalidate drops this process's cache. After an admin edit call Bump instead, which also
// tells the other instances.
func (p *Places) Invalidate() {
	p.mu.Lock()
	p.loadedAt = time.Time{}
	p.mu.Unlock()
}

func (p *Places) ensure(ctx context.Context) {
	if p.db == nil {
		return
	}
	p.mu.RLock()
	fresh := p.states != nil && time.Since(p.loadedAt) < p.ttl
	known, have := p.version, p.haveVersion
	p.mu.RUnlock()
	var v int64
	var vok bool
	if fresh {
		if v, vok = p.checkVersion(ctx); !vok || (have && v == known) {
			return
		}
	} else {
		v, vok = p.readVersion(ctx)
	}
	states := map[int64]State{}
	districts := map[int64]District{}
	rows, err := p.db.QueryContext(ctx, `SELECT id, code, name, is_ut FROM states WHERE active = 1`)
	if err != nil {
		return
	}
	for rows.Next() {
		var s State
		if rows.Scan(&s.ID, &s.Code, &s.Name, &s.IsUT) == nil {
			states[s.ID] = s
		}
	}
	rows.Close()
	rows, err = p.db.QueryContext(ctx, `SELECT id, state_id, name, COALESCE(aliases, JSON_ARRAY()), active FROM districts`)
	if err != nil {
		return
	}
	for rows.Next() {
		var d District
		var aliases string
		if rows.Scan(&d.ID, &d.StateID, &d.Name, &aliases, &d.Active) == nil {
			d.Aliases = parseAliases(aliases)
			districts[d.ID] = d
		}
	}
	rows.Close()
	p.mu.Lock()
	p.states, p.districts, p.loadedAt = states, districts, time.Now()
	p.version, p.haveVersion = v, vok
	p.mu.Unlock()
}

// StateName returns the state name or "".
func (p *Places) StateName(ctx context.Context, id int64) string {
	p.ensure(ctx)
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.states[id].Name
}

// DistrictName returns the district name or "".
func (p *Places) DistrictName(ctx context.Context, id int64) string {
	p.ensure(ctx)
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.districts[id].Name
}

// State returns a state (ok=false if unknown).
func (p *Places) State(ctx context.Context, id int64) (State, bool) {
	p.ensure(ctx)
	p.mu.RLock()
	defer p.mu.RUnlock()
	s, ok := p.states[id]
	return s, ok
}

// District returns a district (ok=false if unknown).
func (p *Places) District(ctx context.Context, id int64) (District, bool) {
	p.ensure(ctx)
	p.mu.RLock()
	defer p.mu.RUnlock()
	d, ok := p.districts[id]
	return d, ok
}

// MatchDistrict finds an active district by state name + district name (or alias),
// case/space/punctuation-insensitive. Used to map the DigiLocker Aadhaar address district to
// a district id; the address itself is never stored. ok=false when nothing matches.
func (p *Places) MatchDistrict(ctx context.Context, stateName, districtName string) (District, bool) {
	p.ensure(ctx)
	p.mu.RLock()
	defer p.mu.RUnlock()
	sn, dn := norm(stateName), norm(districtName)
	if dn == "" {
		return District{}, false
	}
	var stateID int64
	for _, s := range p.states {
		if norm(s.Name) == sn {
			stateID = s.ID
			break
		}
	}
	if stateID == 0 {
		return District{}, false
	}
	for _, d := range p.districts {
		if d.StateID != stateID || !d.Active {
			continue
		}
		if norm(d.Name) == dn {
			return d, true
		}
		for _, a := range d.Aliases {
			if norm(a) == dn {
				return d, true
			}
		}
	}
	return District{}, false
}

func norm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return strings.TrimPrefix(b.String(), "the")
}

func parseAliases(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.Trim(strings.TrimSpace(part), `"`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// NewWithPlaces builds a Checker with a given places cache (tests: TestPlaces()).
func NewWithPlaces(d *sql.DB, st *settings.Store, p *Places) *Checker {
	return &Checker{db: d, settings: st, places: p}
}

// Test fixture ids (match the 0008 seed order): Tamil Nadu and two of its districts, plus Kerala.
const (
	TestStateTN        = 23
	TestStateKL        = 12
	TestDistrictThanj  = 26
	TestDistrictCoimb  = 4
	TestDistrictErnaku = 9001
)

// TestPlaces is a small static cache for tests in any package.
func TestPlaces() *Places {
	return NewStaticPlaces(
		[]State{{ID: TestStateTN, Code: "TN", Name: "Tamil Nadu"}, {ID: TestStateKL, Code: "KL", Name: "Kerala"}},
		[]District{
			{ID: TestDistrictThanj, StateID: TestStateTN, Name: "Thanjavur", Aliases: []string{"Tanjore"}, Active: true},
			{ID: TestDistrictCoimb, StateID: TestStateTN, Name: "Coimbatore", Aliases: []string{"Kovai"}, Active: true},
			{ID: TestDistrictErnaku, StateID: TestStateKL, Name: "Ernakulam", Active: true},
		})
}
