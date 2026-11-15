package event

import (
	"context"
	"time"

	"athlix/backend/internal/platform/eligibility"
)

// Location eligibility glue. Every decision is made by platform/eligibility (Rule.Validate,
// Evaluate, Filter, Describe, Tag); this file only maps rows and inputs to its types.

func ruleOf(r rec) eligibility.Rule {
	rule := eligibility.Rule{Type: r.ElType, RequireVerified: r.RequireVerified}
	if r.ElScope != nil {
		rule.Scope = *r.ElScope
	}
	if r.ElDistrictID != nil {
		rule.DistrictID = *r.ElDistrictID
	}
	if r.ElStateID != nil {
		rule.StateID = *r.ElStateID
	}
	return rule
}

// setRule writes a validated rule onto the row.
func setRule(r *rec, rule eligibility.Rule) {
	r.ElType, r.RequireVerified = rule.Type, rule.RequireVerified
	r.ElScope, r.ElDistrictID, r.ElStateID = nil, nil, nil
	if rule.Type == eligibility.TypeOpen {
		r.RequireVerified = true
		return
	}
	sc := rule.Scope
	r.ElScope = &sc
	if rule.DistrictID > 0 {
		d := rule.DistrictID
		r.ElDistrictID = &d
	}
	if rule.StateID > 0 {
		st := rule.StateID
		r.ElStateID = &st
	}
}

// buildRule turns organiser input into a rule and validates it: Rule.Validate for the shape,
// then the places cache for active ids. For district scope the state is derived from the
// district (never taken from the client).
func (s *Service) buildRule(ctx context.Context, in EligibilityInput) (eligibility.Rule, map[string]string) {
	rule := eligibility.Rule{Type: in.Type, Scope: in.Scope, DistrictID: in.DistrictID, StateID: in.StateID, RequireVerified: true}
	if rule.Type == "" {
		rule.Type = eligibility.TypeOpen
	}
	if in.RequireVerified != nil {
		rule.RequireVerified = *in.RequireVerified
	}
	if rule.Scope == eligibility.ScopeDistrict {
		rule.StateID = 0
	}
	fields := rule.Validate()
	if len(fields) > 0 {
		return rule, fields
	}
	places := s.d.Eligibility.Places()
	switch rule.Scope {
	case eligibility.ScopeDistrict:
		d, ok := places.District(ctx, rule.DistrictID)
		if !ok || !d.Active {
			fields["eligibility.districtId"] = "is not an active district"
			return rule, fields
		}
		rule.StateID = d.StateID
	case eligibility.ScopeState:
		if places.StateName(ctx, rule.StateID) == "" {
			fields["eligibility.stateId"] = "is not an active state"
		}
	}
	return rule, fields
}

// ruleView is EligibilityRule (names + summary).
func (s *Service) ruleView(ctx context.Context, r rec) EligibilityRule {
	rule := ruleOf(r)
	v := EligibilityRule{Type: rule.Type, RequireVerified: rule.RequireVerified, Scope: r.ElScope}
	places := s.d.Eligibility.Places()
	if rule.DistrictID > 0 {
		if d, ok := places.District(ctx, rule.DistrictID); ok {
			v.District = &District{ID: d.ID, StateID: d.StateID, Name: d.Name}
		}
	}
	if rule.StateID > 0 {
		if st, ok := places.State(ctx, rule.StateID); ok {
			v.State = &State{ID: st.ID, Code: st.Code, Name: st.Name, IsUT: st.IsUT}
		} else {
			v.State = &State{ID: rule.StateID}
		}
	}
	v.Summary = eligibility.Describe(rule, s.d.Eligibility.PlaceFor(ctx, rule))
	return v
}

// viewer holds the caller's identities, loaded at most once per request.
type viewer struct {
	id      int64
	enabled bool
	loaded  bool
	ids     []eligibility.Identity
}

func (s *Service) newViewer(ctx context.Context, uid int64) *viewer {
	return &viewer{id: uid, enabled: s.d.Eligibility.Enabled(ctx)}
}

// identities loads the caller's identities once (nil when the feature is off).
func (s *Service) identities(ctx context.Context, v *viewer) ([]eligibility.Identity, error) {
	if !v.enabled || v.loaded {
		return v.ids, nil
	}
	ids, err := s.d.Eligibility.Load(ctx, nil, v.id)
	if err != nil {
		return nil, err
	}
	v.ids, v.loaded = ids, true
	return ids, nil
}

// feedFilter is the eligibility SQL for `events e` ("" when the feature is off). It is
// platform/eligibility's FeedFilter, sharing the identities loaded for myEligibility.
func (s *Service) feedFilter(ctx context.Context, v *viewer, now time.Time) (string, []any, error) {
	if !v.enabled {
		return "", nil, nil
	}
	ids, err := s.identities(ctx, v)
	if err != nil {
		return "", nil, err
	}
	cond, args := eligibility.Filter("e", ids, now)
	return cond, args, nil
}

// evaluate decides one event for the viewer (switch off = open).
func (s *Service) evaluate(ctx context.Context, v *viewer, r rec, now time.Time) (eligibility.Result, error) {
	rule := ruleOf(r)
	if !v.enabled || rule.Type == eligibility.TypeOpen {
		return eligibility.Evaluate(eligibility.Rule{Type: eligibility.TypeOpen}, nil, now), nil
	}
	ids, err := s.identities(ctx, v)
	if err != nil {
		return eligibility.Result{}, err
	}
	return eligibility.Evaluate(rule, ids, now), nil
}

func (s *Service) myEligibility(ctx context.Context, res eligibility.Result) *MyEligibility {
	m := &MyEligibility{Eligible: res.Eligible, Pending: res.Pending, Action: res.Action}
	if m.Action == "" {
		m.Action = eligibility.ActionNone
	}
	if res.Via != "" {
		via := res.Via
		m.Via = &via
	}
	if res.Reason != "" {
		reason := res.Reason
		m.Reason = &reason
	}
	if res.Eligible && res.Via != eligibility.TypeOpen {
		place := s.d.Eligibility.Places().DistrictName(ctx, res.DistrictID)
		if t := eligibility.Tag(res, place); t != "" {
			m.Tag = &t
		}
	}
	return m
}

// Preview is GET /events/eligibility/preview.
func (s *Service) Preview(ctx context.Context, category string, in EligibilityInput) EligibilityPreview {
	fields := map[string]string{}
	if category == "" {
		category = "private"
	}
	if category != "private" && category != "government" {
		fields["category"] = "must be government or private"
	}
	rule, rf := s.buildRule(ctx, in)
	for k, v := range rf {
		fields[k] = v
	}
	out := EligibilityPreview{Valid: len(fields) == 0, Fields: fields}
	if len(rf) == 0 {
		out.Summary = eligibility.Describe(rule, s.d.Eligibility.PlaceFor(ctx, rule))
	}
	if category == "government" {
		out.SuggestedType = s.d.Settings.String(ctx, "event.eligibility_default_government", defEligibilityGov)
	} else {
		out.SuggestedType = s.d.Settings.String(ctx, "event.eligibility_default_private", defEligibilityPriv)
	}
	return out
}
