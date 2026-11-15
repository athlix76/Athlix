// Package requirements answers "must a user with role R complete requirement X before action G?"
// from the verification_requirements table (admin-editable, see docs/config-system.md).
//
// It is the ONE source of truth for gating: onboarding (gate app_access), event creation
// (create_event), consultation listing (offer_consultation) and community creation
// (create_community). Never hard-code these rules in a module.
package requirements

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// Requirements.
const (
	DigiLocker             = "digilocker"
	GuardianConsentIfMinor = "guardian_consent_if_minor"
	OrganiserApproval      = "organiser_approval"
	CoachProSubscription   = "coach_pro_subscription"
)

// Gates.
const (
	GateAppAccess         = "app_access"
	GateCreateEvent       = "create_event"
	GateOfferConsultation = "offer_consultation"
	GateCreateCommunity   = "create_community"
)

type key struct{ role, requirement, gate string }

// Checker caches the table for ttl.
type Checker struct {
	db  *sql.DB
	ttl time.Duration

	mu     sync.Mutex
	rows   map[key]bool
	loaded time.Time
	static bool
}

// New builds a DB-backed Checker.
func New(d *sql.DB, ttl time.Duration) *Checker { return &Checker{db: d, ttl: ttl} }

// NewStatic is for tests: rows maps "role/requirement/gate" to is_required.
func NewStatic(rows map[string]bool) *Checker {
	c := &Checker{rows: map[key]bool{}, static: true}
	for k, v := range rows {
		var parts [3]string
		i := 0
		for _, ch := range k {
			if ch == '/' && i < 2 {
				i++
				continue
			}
			parts[i] += string(ch)
		}
		c.rows[key{parts[0], parts[1], parts[2]}] = v
	}
	return c
}

// Invalidate drops the cache (called by the admin module after an edit).
func (c *Checker) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.static {
		c.loaded = time.Time{}
	}
}

func (c *Checker) snapshot(ctx context.Context) map[key]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.static || (c.rows != nil && time.Since(c.loaded) < c.ttl) {
		return c.rows
	}
	rows, err := c.db.QueryContext(ctx, `SELECT role, requirement, gates, is_required FROM verification_requirements`)
	if err != nil {
		return c.rows // keep last good snapshot (nil => nothing required => callers fail safe below)
	}
	defer rows.Close()
	m := map[key]bool{}
	for rows.Next() {
		var k key
		var req bool
		if rows.Scan(&k.role, &k.requirement, &k.gate, &req) == nil {
			m[k] = req
		}
	}
	if rows.Err() == nil {
		c.rows, c.loaded = m, time.Now()
	}
	return c.rows
}

// Required reports whether role must complete requirement before gate.
// If the table could never be loaded, DigiLocker and guardian consent default to required
// (fail safe); everything else defaults to not required.
func (c *Checker) Required(ctx context.Context, role, requirement, gate string) bool {
	m := c.snapshot(ctx)
	if m == nil {
		return requirement == DigiLocker || requirement == GuardianConsentIfMinor
	}
	return m[key{role, requirement, gate}]
}

// RequiredForAny reports whether any of roles must complete requirement before gate.
func (c *Checker) RequiredForAny(ctx context.Context, roles []string, requirement, gate string) bool {
	for _, r := range roles {
		if c.Required(ctx, r, requirement, gate) {
			return true
		}
	}
	return false
}
