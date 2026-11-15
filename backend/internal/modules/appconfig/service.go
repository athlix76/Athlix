package appconfig

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/sport"
	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/eligibility"
	"athlix/backend/internal/platform/httpx"
)

// enumKeys are the enum-like string keys of docs/config-system.md and their allowed values.
var enumKeys = map[string][]string{
	"auth.sms_provider":                    {"stub", "msg91"},
	"verification.provider":                {"digilocker_direct", "aggregator", "mock"},
	"minor.dm_policy":                      {"mutual_follow", "none", "anyone"},
	"minor.profile_default_visibility":     {"public", "followers", "private"},
	"event.show_contact":                   {"always", "after_registration"},
	"event.eligibility_default_government": {eligibility.TypeOpen, eligibility.TypeNative, eligibility.TypeResidence, eligibility.TypeEither},
	"event.eligibility_default_private":    {eligibility.TypeOpen, eligibility.TypeNative, eligibility.TypeResidence, eligibility.TypeEither},
	"listing.payment_provider":             {"razorpay", "mock"},
}

// mockRefusedInProd are provider keys whose "mock" value is refused when ENV=prod.
var mockRefusedInProd = []string{"verification.provider", "listing.payment_provider"}

var (
	versionKeys = []string{"app.min_version_android", "app.min_version_ios"}
	emailKeys   = []string{"app.support_email"}
	versionRE   = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}$`)
	codeRE      = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
	stepCodeRE  = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)
	fieldKeyRE  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
)

// Size limits follow the column sizes in migrations 0002 / 0006.
const (
	historyLimit     = 100
	maxStringValue   = 1000
	maxListItems     = 100
	maxJSONValue     = 64 << 10
	maxPolicyContent = 1 << 20
	maxTemplateField = 50
)

// Service holds the appconfig rules.
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

func (s *Service) now() time.Time { return s.d.Now().UTC() }

func validation(field, msg string) error { return httpx.Validation(map[string]string{field: msg}) }

// text trims v and checks it is 1..max runes.
func text(fields map[string]string, name, v string, max int) string {
	v = strings.TrimSpace(v)
	if v == "" || utf8.RuneCountInString(v) > max {
		fields[name] = "must be 1-" + strconv.Itoa(max) + " characters"
	}
	return v
}

// ---- public ----

// Bootstrap is GET /bootstrap.
func (s *Service) Bootstrap(ctx context.Context) (Bootstrap, error) {
	pols, err := s.repo.activePolicies(ctx)
	if err != nil {
		return Bootstrap{}, err
	}
	sports, err := sport.Catalogue(ctx, s.d.DB, false)
	if err != nil {
		return Bootstrap{}, err
	}
	st := s.d.Settings
	return Bootstrap{
		Config:   st.Public(ctx),
		Policies: pols,
		Sports:   sports,
		MinVersion: MinVersion{
			Android: st.String(ctx, "app.min_version_android", "1.0.0"),
			IOS:     st.String(ctx, "app.min_version_ios", "1.0.0"),
		},
		Maintenance: st.Bool(ctx, "app.maintenance_mode", false),
		ServerTime:  s.now(),
	}, nil
}

// Policy is GET /policies/:docType (the active version).
func (s *Service) Policy(ctx context.Context, docType string) (PolicyDocument, error) {
	if !slices.Contains(DocTypes, docType) {
		return PolicyDocument{}, httpx.NotFound("Policy")
	}
	p, err := s.repo.activePolicy(ctx, docType)
	if db.IsNoRows(err) {
		return PolicyDocument{}, httpx.NotFound("Policy")
	}
	return p.PolicyDocument, err
}

// ---- app_config ----

// ConfigList is GET /admin/config. It reads the table, not the cache, so the editor is fresh.
func (s *Service) ConfigList(ctx context.Context, category string) ([]ConfigEntry, error) {
	if len(category) > 32 {
		return nil, validation("category", "is too long")
	}
	return s.repo.configList(ctx, category)
}

// ConfigHistory is GET /admin/config/:key/history (newest first).
func (s *Service) ConfigHistory(ctx context.Context, key string) ([]ConfigChange, error) {
	if len(key) > 96 {
		return nil, httpx.NotFound("Config key")
	}
	ok, err := s.repo.configExists(ctx, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, httpx.NotFound("Config key")
	}
	return s.repo.configHistory(ctx, key, historyLimit)
}

// validateValue checks raw against valueType and the per-key rules, and returns it compacted.
func (s *Service) validateValue(key, valueType string, raw json.RawMessage) (json.RawMessage, string) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, "is required"
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return nil, "must be valid JSON"
	}
	switch valueType {
	case "int":
		n, ok := v.(json.Number)
		if !ok {
			return nil, "must be an integer"
		}
		i, err := strconv.ParseInt(n.String(), 10, 64)
		if err != nil || i < 0 {
			return nil, "must be a non-negative integer"
		}
	case "decimal":
		n, ok := v.(json.Number)
		if !ok {
			return nil, "must be a number"
		}
		f, err := strconv.ParseFloat(n.String(), 64)
		if err != nil || f < 0 {
			return nil, "must be a non-negative number"
		}
	case "bool":
		if _, ok := v.(bool); !ok {
			return nil, "must be true or false"
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			return nil, "must be a string"
		}
		if utf8.RuneCountInString(str) > maxStringValue {
			return nil, "is too long"
		}
		if allowed, ok := enumKeys[key]; ok && !slices.Contains(allowed, str) {
			return nil, "must be one of " + strings.Join(allowed, ", ")
		}
		if str == "mock" && slices.Contains(mockRefusedInProd, key) && s.d.Cfg.Env == config.EnvProd {
			return nil, "mock is not allowed in production"
		}
		if slices.Contains(versionKeys, key) && !versionRE.MatchString(str) {
			return nil, "must look like 1.2.3"
		}
		if slices.Contains(emailKeys, key) {
			if a, err := mail.ParseAddress(str); err != nil || a.Address != str {
				return nil, "must be a valid email address"
			}
		}
	case "string_list":
		list, ok := v.([]any)
		if !ok || len(list) > maxListItems {
			return nil, "must be a list of at most 100 strings"
		}
		for _, it := range list {
			str, ok := it.(string)
			if !ok || strings.TrimSpace(str) == "" || utf8.RuneCountInString(str) > 200 {
				return nil, "must be a list of non-empty strings"
			}
		}
	case "json":
		if _, ok := v.(map[string]any); !ok || len(raw) > maxJSONValue {
			return nil, "must be a JSON object"
		}
	default:
		return nil, "has an unknown value type"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, "must be valid JSON"
	}
	return buf.Bytes(), ""
}

// PutConfig is PUT /admin/config/:key with optimistic locking on version.
func (s *Service) PutConfig(ctx context.Context, a actor, key string, in putConfigBody) (ConfigEntry, error) {
	if in.Version <= 0 {
		return ConfigEntry{}, validation("version", "is required")
	}
	if len(key) > 96 {
		return ConfigEntry{}, httpx.NotFound("Config key")
	}
	var out ConfigEntry
	changed := false
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.configForUpdate(ctx, tx, key)
		if db.IsNoRows(err) {
			return httpx.NotFound("Config key")
		}
		if err != nil {
			return err
		}
		val, msg := s.validateValue(key, cur.ValueType, in.Value)
		if msg != "" {
			return validation("value", msg)
		}
		if in.Version != cur.Version {
			return httpx.Conflict("This setting was changed by someone else. Reload and try again.")
		}
		var old bytes.Buffer
		if json.Compact(&old, cur.Value) == nil && bytes.Equal(old.Bytes(), val) {
			out = cur
			return nil
		}
		next := cur
		next.Value, next.Version, next.UpdatedAt = json.RawMessage(val), cur.Version+1, s.now()
		if err := s.repo.updateConfig(ctx, tx, key, next.Value, next.Version, a.id, next.UpdatedAt); err != nil {
			return err
		}
		if err := audit.Config(ctx, tx, "app_config", key, cur.Value, next.Value, a.id); err != nil {
			return err
		}
		out, changed = next, true
		return a.log(tx, "config.update", "app_config", key, map[string]any{"version": next.Version})
	})
	if err == nil && changed {
		s.d.Settings.Invalidate()
	}
	return out, err
}

// ---- policies ----

// Policies is GET /admin/policies.
func (s *Service) Policies(ctx context.Context, docType string) ([]AdminPolicy, error) {
	if docType != "" && !slices.Contains(DocTypes, docType) {
		return nil, validation("docType", "is not a known policy type")
	}
	return s.repo.policies(ctx, docType)
}

// CreatePolicy is POST /admin/policies: a new inactive version (max + 1) of docType.
func (s *Service) CreatePolicy(ctx context.Context, a actor, in createPolicyBody) (AdminPolicy, error) {
	fields := map[string]string{}
	if !slices.Contains(DocTypes, in.DocType) {
		fields["docType"] = "is not a known policy type"
	}
	in.Title = text(fields, "title", in.Title, 120)
	if strings.TrimSpace(in.ContentMd) == "" || len(in.ContentMd) > maxPolicyContent {
		fields["contentMd"] = "is required (at most 1 MB)"
	}
	if len(fields) > 0 {
		return AdminPolicy{}, httpx.Validation(fields)
	}
	var out AdminPolicy
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		v, err := s.repo.maxPolicyVersion(ctx, tx, in.DocType)
		if err != nil {
			return err
		}
		now := s.now()
		id, err := s.repo.insertPolicy(ctx, tx, in, v+1, a.id, now)
		if db.IsDuplicate(err) {
			return httpx.Conflict("A new version of this policy was just created. Try again.")
		}
		if err != nil {
			return err
		}
		out = AdminPolicy{PolicyDocument: PolicyDocument{ID: id, DocType: in.DocType, Version: v + 1, Title: in.Title, ContentMd: in.ContentMd,
			RequiresAcceptance: in.RequiresAcceptance, EffectiveFrom: now}, Active: false, CreatedAt: now}
		rec := strconv.FormatInt(id, 10)
		snap := map[string]any{"docType": in.DocType, "version": v + 1, "title": in.Title, "requiresAcceptance": in.RequiresAcceptance, "active": false}
		if err := audit.Config(ctx, tx, "policy_documents", rec, nil, snap, a.id); err != nil {
			return err
		}
		return a.log(tx, "policy.create", "policy_document", rec, map[string]any{"version": v + 1})
	})
	return out, err
}

// ActivatePolicy is POST /admin/policies/:id/activate. The previous active version of the
// same type is deactivated in the same transaction. Activating an active version is a no-op.
func (s *Service) ActivatePolicy(ctx context.Context, a actor, id int64) (PolicyDocument, error) {
	var out PolicyDocument
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		p, err := s.repo.policyForUpdate(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("Policy")
		}
		if err != nil {
			return err
		}
		if p.Active {
			out = p.PolicyDocument
			return nil
		}
		prev, err := s.repo.activePolicyIDs(ctx, tx, p.DocType)
		if err != nil {
			return err
		}
		now := s.now()
		if err := s.repo.activatePolicy(ctx, tx, id, p.DocType, now); err != nil {
			return err
		}
		for _, pid := range prev {
			if err := audit.Config(ctx, tx, "policy_documents", strconv.FormatInt(pid, 10), map[string]any{"active": true}, map[string]any{"active": false}, a.id); err != nil {
				return err
			}
		}
		if err := audit.Config(ctx, tx, "policy_documents", strconv.FormatInt(id, 10), map[string]any{"active": false}, map[string]any{"active": true}, a.id); err != nil {
			return err
		}
		p.EffectiveFrom = now
		out = p.PolicyDocument
		return a.log(tx, "policy.activate", "policy_document", strconv.FormatInt(id, 10), map[string]any{"deactivatedIds": prev})
	})
	return out, err
}

// ---- verification requirements ----

// Requirements is GET /admin/verification-requirements.
func (s *Service) Requirements(ctx context.Context) ([]Requirement, error) {
	return s.repo.requirements(ctx)
}

// CreateRequirement is POST /admin/verification-requirements.
func (s *Service) CreateRequirement(ctx context.Context, a actor, in createReqBody) (Requirement, error) {
	fields := map[string]string{}
	if !slices.Contains(ReqRoles, in.Role) {
		fields["role"] = "must be one of " + strings.Join(ReqRoles, ", ")
	}
	if !slices.Contains(ReqRequirements, in.Requirement) {
		fields["requirement"] = "must be one of " + strings.Join(ReqRequirements, ", ")
	}
	if !slices.Contains(ReqGates, in.Gates) {
		fields["gates"] = "must be one of " + strings.Join(ReqGates, ", ")
	}
	if in.IsRequired == nil {
		fields["isRequired"] = "is required"
	}
	if len(fields) > 0 {
		return Requirement{}, httpx.Validation(fields)
	}
	row := Requirement{Role: in.Role, Requirement: in.Requirement, IsRequired: *in.IsRequired, Gates: in.Gates}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		id, err := s.repo.insertRequirement(ctx, tx, row)
		if db.IsDuplicate(err) {
			return httpx.Conflict("This requirement already exists for that role and gate.")
		}
		if err != nil {
			return err
		}
		row.ID = id
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "verification_requirements", rec, nil, row, a.id); err != nil {
			return err
		}
		return a.log(tx, "verification_requirement.create", "verification_requirement", rec, map[string]any{"isRequired": row.IsRequired})
	})
	if err == nil {
		s.d.Requirements.Invalidate()
	}
	return row, err
}

// UpdateRequirement is PUT /admin/verification-requirements/:id.
func (s *Service) UpdateRequirement(ctx context.Context, a actor, id int64, in updateReqBody) (Requirement, error) {
	if in.IsRequired == nil {
		return Requirement{}, validation("isRequired", "is required")
	}
	var out Requirement
	changed := false
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.requirementForUpdate(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("Requirement")
		}
		if err != nil {
			return err
		}
		out = cur
		if cur.IsRequired == *in.IsRequired {
			return nil
		}
		next := cur
		next.IsRequired = *in.IsRequired
		if err := s.repo.setRequirement(ctx, tx, id, next.IsRequired); err != nil {
			return err
		}
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "verification_requirements", rec, cur, next, a.id); err != nil {
			return err
		}
		out, changed = next, true
		return a.log(tx, "verification_requirement.update", "verification_requirement", rec, map[string]any{"isRequired": next.IsRequired})
	})
	if err == nil && changed {
		s.d.Requirements.Invalidate()
	}
	return out, err
}

// ---- organiser steps ----

// Steps is GET /admin/organiser-steps (all, including inactive).
func (s *Service) Steps(ctx context.Context) ([]OrganiserStep, error) { return s.repo.steps(ctx) }

// applyStep merges a partial body into st and validates the result.
func applyStep(st OrganiserStep, in stepBody, create bool) (OrganiserStep, error) {
	fields := map[string]string{}
	if create {
		code := ""
		if in.StepCode != nil {
			code = strings.TrimSpace(*in.StepCode)
		}
		if !stepCodeRE.MatchString(code) {
			fields["stepCode"] = "must be 2-48 lowercase letters, digits or underscores"
		}
		st.StepCode = code
		if in.Title == nil {
			fields["title"] = "is required"
		}
		if in.InputType == nil {
			fields["inputType"] = "is required"
		}
	} else if in.StepCode != nil && *in.StepCode != st.StepCode {
		fields["stepCode"] = "cannot be changed (submissions refer to it)"
	}
	if in.Title != nil {
		st.Title = text(fields, "title", *in.Title, 120)
	}
	if in.Description != nil {
		d := strings.TrimSpace(*in.Description)
		switch {
		case utf8.RuneCountInString(d) > 500:
			fields["description"] = "must be at most 500 characters"
		case d == "":
			st.Description = nil
		default:
			st.Description = &d
		}
	}
	if in.InputType != nil {
		if !slices.Contains(StepInputTypes, *in.InputType) {
			fields["inputType"] = "must be one of " + strings.Join(StepInputTypes, ", ")
		}
		st.InputType = *in.InputType
	}
	if in.IsRequired != nil {
		st.IsRequired = *in.IsRequired
	}
	if in.SortOrder != nil {
		st.SortOrder = *in.SortOrder
	}
	if in.Active != nil {
		st.Active = *in.Active
	}
	if len(fields) > 0 {
		return st, httpx.Validation(fields)
	}
	return st, nil
}

// CreateStep is POST /admin/organiser-steps. isRequired and active default to true.
func (s *Service) CreateStep(ctx context.Context, a actor, in stepBody) (OrganiserStep, error) {
	st, err := applyStep(OrganiserStep{IsRequired: true, Active: true}, in, true)
	if err != nil {
		return OrganiserStep{}, err
	}
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		id, err := s.repo.insertStep(ctx, tx, st)
		if db.IsDuplicate(err) {
			return httpx.Conflict("A step with this code already exists.")
		}
		if err != nil {
			return err
		}
		st.ID = id
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "organiser_verification_steps", rec, nil, st, a.id); err != nil {
			return err
		}
		return a.log(tx, "organiser_step.create", "organiser_step", rec, nil)
	})
	return st, err
}

// UpdateStep is PUT /admin/organiser-steps/:id (partial).
func (s *Service) UpdateStep(ctx context.Context, a actor, id int64, in stepBody) (OrganiserStep, error) {
	var out OrganiserStep
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.stepForUpdate(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("Step")
		}
		if err != nil {
			return err
		}
		next, err := applyStep(cur, in, false)
		if err != nil {
			return err
		}
		if err := s.repo.updateStep(ctx, tx, next); err != nil {
			return err
		}
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "organiser_verification_steps", rec, cur, next, a.id); err != nil {
			return err
		}
		out = next
		return a.log(tx, "organiser_step.update", "organiser_step", rec, map[string]any{"active": next.Active, "isRequired": next.IsRequired})
	})
	return out, err
}

// ---- sports ----

// AdminSports is GET /admin/sports (including inactive rows).
func (s *Service) AdminSports(ctx context.Context) ([]sport.Sport, error) {
	return sport.Catalogue(ctx, s.d.DB, true)
}

// oneSport reloads the admin view of sport id after a write.
func (s *Service) oneSport(ctx context.Context, id int64) (sport.Sport, error) {
	all, err := sport.Catalogue(ctx, s.d.DB, true)
	if err != nil {
		return sport.Sport{}, err
	}
	for _, sp := range all {
		if sp.ID == id {
			return sp, nil
		}
	}
	return sport.Sport{}, httpx.NotFound("Sport")
}

// CreateSport is POST /admin/sports.
func (s *Service) CreateSport(ctx context.Context, a actor, in createSportBody) (sport.Sport, error) {
	fields := map[string]string{}
	code := strings.TrimSpace(in.Code)
	if !codeRE.MatchString(code) {
		fields["code"] = "must be 2-32 lowercase letters, digits or underscores"
	}
	name := text(fields, "name", in.Name, 64)
	if len(fields) > 0 {
		return sport.Sport{}, httpx.Validation(fields)
	}
	row := sportRow{Code: code, Name: name, IsTeamSport: in.IsTeamSport, SortOrder: in.SortOrder, Active: true}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		id, err := s.repo.insertSport(ctx, tx, row)
		if db.IsDuplicate(err) {
			return httpx.Conflict("A sport with this code already exists.")
		}
		if err != nil {
			return err
		}
		row.ID = id
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "sports", rec, nil, row, a.id); err != nil {
			return err
		}
		return a.log(tx, "sport.create", "sport", rec, nil)
	})
	if err != nil {
		return sport.Sport{}, err
	}
	active, sort := true, row.SortOrder
	return sport.Sport{ID: row.ID, Code: row.Code, Name: row.Name, IsTeamSport: row.IsTeamSport,
		Positions: []sport.Position{}, StatTemplates: []sport.StatTemplate{}, Active: &active, SortOrder: &sort}, nil
}

// UpdateSport is PUT /admin/sports/:id. The code never changes (profiles refer to the id).
func (s *Service) UpdateSport(ctx context.Context, a actor, id int64, in updateSportBody) (sport.Sport, error) {
	fields := map[string]string{}
	var name string
	if in.Name != nil {
		name = text(fields, "name", *in.Name, 64)
	}
	if len(fields) > 0 {
		return sport.Sport{}, httpx.Validation(fields)
	}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.sportForUpdate(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("Sport")
		}
		if err != nil {
			return err
		}
		next := cur
		if in.Name != nil {
			next.Name = name
		}
		if in.IsTeamSport != nil {
			next.IsTeamSport = *in.IsTeamSport
		}
		if in.SortOrder != nil {
			next.SortOrder = *in.SortOrder
		}
		if in.Active != nil {
			next.Active = *in.Active
		}
		if next == cur {
			return nil
		}
		if err := s.repo.updateSport(ctx, tx, next); err != nil {
			return err
		}
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "sports", rec, cur, next, a.id); err != nil {
			return err
		}
		return a.log(tx, "sport.update", "sport", rec, map[string]any{"active": next.Active})
	})
	if err != nil {
		return sport.Sport{}, err
	}
	return s.oneSport(ctx, id)
}

// CreatePosition is POST /admin/sports/:id/positions.
func (s *Service) CreatePosition(ctx context.Context, a actor, sportID int64, in createPositionBody) (sport.Position, error) {
	fields := map[string]string{}
	code := strings.TrimSpace(in.Code)
	if !codeRE.MatchString(code) {
		fields["code"] = "must be 2-32 lowercase letters, digits or underscores"
	}
	name := text(fields, "name", in.Name, 64)
	if !slices.Contains(AppliesTo, in.AppliesTo) {
		fields["appliesTo"] = "must be one of " + strings.Join(AppliesTo, ", ")
	}
	if len(fields) > 0 {
		return sport.Position{}, httpx.Validation(fields)
	}
	row := positionRow{SportID: sportID, Code: code, Name: name, AppliesTo: in.AppliesTo, SortOrder: in.SortOrder, Active: true}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.repo.sportForUpdate(ctx, tx, sportID); db.IsNoRows(err) {
			return httpx.NotFound("Sport")
		} else if err != nil {
			return err
		}
		id, err := s.repo.insertPosition(ctx, tx, row)
		if db.IsDuplicate(err) {
			return httpx.Conflict("This sport already has a position with this code.")
		}
		if err != nil {
			return err
		}
		row.ID = id
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "sport_positions", rec, nil, row, a.id); err != nil {
			return err
		}
		return a.log(tx, "sport_position.create", "sport_position", rec, map[string]any{"sportId": sportID})
	})
	return positionOut(row), err
}

func positionOut(p positionRow) sport.Position {
	active, sort := p.Active, p.SortOrder
	return sport.Position{ID: p.ID, Code: p.Code, Name: p.Name, AppliesTo: p.AppliesTo, Active: &active, SortOrder: &sort}
}

// UpdatePosition is PUT /admin/positions/:id.
func (s *Service) UpdatePosition(ctx context.Context, a actor, id int64, in updatePositionBody) (sport.Position, error) {
	fields := map[string]string{}
	var name string
	if in.Name != nil {
		name = text(fields, "name", *in.Name, 64)
	}
	if in.AppliesTo != nil && !slices.Contains(AppliesTo, *in.AppliesTo) {
		fields["appliesTo"] = "must be one of " + strings.Join(AppliesTo, ", ")
	}
	if len(fields) > 0 {
		return sport.Position{}, httpx.Validation(fields)
	}
	var out positionRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.positionForUpdate(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("Position")
		}
		if err != nil {
			return err
		}
		next := cur
		if in.Name != nil {
			next.Name = name
		}
		if in.AppliesTo != nil {
			next.AppliesTo = *in.AppliesTo
		}
		if in.SortOrder != nil {
			next.SortOrder = *in.SortOrder
		}
		if in.Active != nil {
			next.Active = *in.Active
		}
		out = next
		if next == cur {
			return nil
		}
		if err := s.repo.updatePosition(ctx, tx, next); err != nil {
			return err
		}
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "sport_positions", rec, cur, next, a.id); err != nil {
			return err
		}
		return a.log(tx, "sport_position.update", "sport_position", rec, map[string]any{"active": next.Active})
	})
	return positionOut(out), err
}

// validateFields checks a stat template field list.
func validateFields(in []sport.StatField) ([]sport.StatField, string) {
	if len(in) == 0 || len(in) > maxTemplateField {
		return nil, "must have 1-50 fields"
	}
	seen := map[string]bool{}
	out := make([]sport.StatField, 0, len(in))
	for _, f := range in {
		f.Key, f.Label = strings.TrimSpace(f.Key), strings.TrimSpace(f.Label)
		if !fieldKeyRE.MatchString(f.Key) || seen[f.Key] {
			return nil, "every key must be unique, lowercase letters, digits or underscores"
		}
		seen[f.Key] = true
		if f.Label == "" || utf8.RuneCountInString(f.Label) > 64 {
			return nil, "every label must be 1-64 characters"
		}
		if !slices.Contains(sport.FieldTypes, f.Type) {
			return nil, "every type must be one of " + strings.Join(sport.FieldTypes, ", ")
		}
		if f.Unit != nil {
			u := strings.TrimSpace(*f.Unit)
			if utf8.RuneCountInString(u) > 16 {
				return nil, "every unit must be at most 16 characters"
			}
			if u == "" {
				f.Unit = nil
			} else {
				f.Unit = &u
			}
		}
		out = append(out, f)
	}
	return out, ""
}

func templateOut(t templateRow) sport.StatTemplate {
	active := t.Active
	return sport.StatTemplate{ID: t.ID, PositionID: t.PositionID, Name: t.Name, Version: t.Version, Fields: t.Fields, Active: &active}
}

// CreateTemplate is POST /admin/sports/:id/stat-templates. positionId must belong to the sport.
func (s *Service) CreateTemplate(ctx context.Context, a actor, sportID int64, in createTemplateBody) (sport.StatTemplate, error) {
	fields := map[string]string{}
	name := text(fields, "name", in.Name, 64)
	fl, msg := validateFields(in.Fields)
	if msg != "" {
		fields["fields"] = msg
	}
	if len(fields) > 0 {
		return sport.StatTemplate{}, httpx.Validation(fields)
	}
	row := templateRow{SportID: sportID, PositionID: in.PositionID, Name: name, Fields: fl, Version: 1, Active: true}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.repo.sportForUpdate(ctx, tx, sportID); db.IsNoRows(err) {
			return httpx.NotFound("Sport")
		} else if err != nil {
			return err
		}
		if in.PositionID != nil {
			ps, err := s.repo.positionSport(ctx, tx, *in.PositionID)
			if db.IsNoRows(err) || (err == nil && ps != sportID) {
				return validation("positionId", "is not a position of this sport")
			}
			if err != nil {
				return err
			}
		}
		id, err := s.repo.insertTemplate(ctx, tx, row)
		if err != nil {
			return err
		}
		row.ID = id
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "stat_templates", rec, nil, row, a.id); err != nil {
			return err
		}
		return a.log(tx, "stat_template.create", "stat_template", rec, map[string]any{"sportId": sportID})
	})
	return templateOut(row), err
}

// UpdateTemplate is PUT /admin/stat-templates/:id. Changing fields bumps version.
func (s *Service) UpdateTemplate(ctx context.Context, a actor, id int64, in updateTemplateBody) (sport.StatTemplate, error) {
	fields := map[string]string{}
	var name string
	if in.Name != nil {
		name = text(fields, "name", *in.Name, 64)
	}
	var fl []sport.StatField
	if in.Fields != nil {
		var msg string
		if fl, msg = validateFields(*in.Fields); msg != "" {
			fields["fields"] = msg
		}
	}
	if len(fields) > 0 {
		return sport.StatTemplate{}, httpx.Validation(fields)
	}
	var out templateRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		cur, err := s.repo.templateForUpdate(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("Stat template")
		}
		if err != nil {
			return err
		}
		next := cur
		changed := false
		if in.Name != nil && name != cur.Name {
			next.Name, changed = name, true
		}
		if in.Active != nil && *in.Active != cur.Active {
			next.Active, changed = *in.Active, true
		}
		if fl != nil {
			oldB, _ := json.Marshal(cur.Fields)
			newB, _ := json.Marshal(fl)
			if !bytes.Equal(oldB, newB) {
				next.Fields, next.Version, changed = fl, cur.Version+1, true
			}
		}
		out = next
		if !changed {
			return nil
		}
		if err := s.repo.updateTemplate(ctx, tx, next); err != nil {
			return err
		}
		rec := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "stat_templates", rec, cur, next, a.id); err != nil {
			return err
		}
		return a.log(tx, "stat_template.update", "stat_template", rec, map[string]any{"version": next.Version, "active": next.Active})
	})
	return templateOut(out), err
}
