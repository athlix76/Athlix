// Package entitlements answers "does this user's active premium plan include benefit X?".
//
// Plans and benefits are DB config (premium_plans, premium_plan_benefits), so what a plan
// includes changes without a deploy. Benefit keys are a closed list (docs/config-system.md).
// Read-only over tables owned by modules/subscription.
package entitlements

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
)

// Benefit keys.
const (
	ConsultationListing   = "consultation_listing"
	ProfileBoost          = "profile_boost"
	AdvancedStats         = "advanced_stats"
	SeeProfileViewers     = "see_profile_viewers"
	AdvancedSearchFilters = "advanced_search_filters"
	ExtraCommunities      = "extra_communities"
	FeaturedEvents        = "featured_events"
	VerifiedCoachBadge    = "verified_coach_badge"
)

// Checker reads active subscriptions.
type Checker struct{ db *sql.DB }

// New builds a Checker.
func New(d *sql.DB) *Checker { return &Checker{db: d} }

// Benefits returns every benefit of the user's active (or grace) subscriptions. If two plans
// grant the same key, the numerically larger value wins.
func (c *Checker) Benefits(ctx context.Context, userID int64) (map[string]json.RawMessage, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT b.benefit_key, b.value
		FROM subscriptions s
		JOIN premium_plans p ON p.id = s.plan_id
		JOIN premium_plan_benefits b ON b.plan_id = p.id
		WHERE s.user_id = ? AND s.status IN ('active','grace') AND s.expires_at > CURRENT_TIMESTAMP(3)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		if old, ok := out[k]; ok && num(old) >= num(v) {
			continue
		}
		out[k] = v
	}
	return out, rows.Err()
}

func num(v json.RawMessage) float64 {
	if string(v) == "true" {
		return 1
	}
	f, _ := strconv.ParseFloat(string(v), 64)
	return f
}

// Has reports whether a boolean benefit is granted (true, or any number > 0).
func (c *Checker) Has(ctx context.Context, userID int64, key string) (bool, error) {
	m, err := c.Benefits(ctx, userID)
	if err != nil {
		return false, err
	}
	v, ok := m[key]
	return ok && num(v) > 0, nil
}

// Int returns a numeric benefit, or def when not granted.
func (c *Checker) Int(ctx context.Context, userID int64, key string, def int) (int, error) {
	m, err := c.Benefits(ctx, userID)
	if err != nil {
		return def, err
	}
	v, ok := m[key]
	if !ok {
		return def, nil
	}
	return int(num(v)), nil
}
