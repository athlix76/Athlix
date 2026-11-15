# appconfig
Runtime configuration without a deploy: the public `GET /bootstrap` and policy pages, and
the admin editors for `app_config`, policy documents, verification requirements, organiser
verification steps and the sports catalogue (sports, positions, stat templates).

## Routes
| Method | Path | Group | Permission |
|---|---|---|---|
| GET | `/bootstrap` | Public | |
| GET | `/policies/:docType` | Public | |
| GET | `/admin/config` | Admin | `config.read` |
| PUT | `/admin/config/:key` | Admin | `config.write` |
| GET | `/admin/config/:key/history` | Admin | `config.read` |
| GET | `/admin/policies` | Admin | `config.read` |
| POST | `/admin/policies` | Admin | `policies.write` |
| POST | `/admin/policies/:id/activate` | Admin | `policies.write` |
| GET | `/admin/verification-requirements` | Admin | `config.read` |
| POST | `/admin/verification-requirements` | Admin | `config.write` |
| PUT | `/admin/verification-requirements/:id` | Admin | `config.write` |
| GET | `/admin/organiser-steps` | Admin | `config.read` |
| POST | `/admin/organiser-steps` | Admin | `config.write` |
| PUT | `/admin/organiser-steps/:id` | Admin | `config.write` |
| GET | `/admin/sports` | Admin | `config.read` |
| POST | `/admin/sports` | Admin | `sports.write` |
| PUT | `/admin/sports/:id` | Admin | `sports.write` |
| POST | `/admin/sports/:id/positions` | Admin | `sports.write` |
| PUT | `/admin/positions/:id` | Admin | `sports.write` |
| POST | `/admin/sports/:id/stat-templates` | Admin | `sports.write` |
| PUT | `/admin/stat-templates/:id` | Admin | `sports.write` |

## Tables
- Owns: `app_config`, `policy_documents`, `verification_requirements`,
  `organiser_verification_steps`.
- Writes (read-only for `sport`): `sports`, `sport_positions`, `stat_templates`.
- Writes audit rows: `config_audit` (via `audit.Config`), `admin_audit_log` (via `audit.Admin`).
- Reads: `admin_users` (name in config history).

## Events
None.

## Jobs
None.

## Config keys
- Reads `app.min_version_android`, `app.min_version_ios`, `app.maintenance_mode` for bootstrap.
- Edits every `app_config` key. Per-key rules live in `service.go`: `enumKeys` (allowed
  values of enum-like strings), `mockRefusedInProd`, `versionKeys`, `emailKeys`.

## Rules
- Bootstrap: `config` is every `is_public = 1` row from `d.Settings`; policies are the
  active versions; sports are the public `sport.Catalogue`.
- Config PUT: `version` must equal the stored version (else 409 CONFLICT); the value must
  match `value_type` (int/decimal non-negative numbers, bool, string, string_list, JSON
  object), else 422. Enum-like strings are checked against `enumKeys`; `mock` providers
  are refused when ENV=prod. A value equal to the stored one is a no-op (no version bump,
  no audit). After a change: `version + 1`, `config_audit` + `admin_audit_log` in the same
  transaction, then `d.Settings.Invalidate()` (other processes catch up within 15 s).
- Policies: `docType` must be in `DocTypes` (includes `location_identity_notice`). A new
  policy is version `MAX(version) + 1` of its type and inactive. Activating deactivates the
  other active version(s) of that type and sets `effective_from` to now; activating an active
  version is a no-op.
- Verification requirements: enum values only, duplicate (role, requirement, gates) = 409;
  every change calls `d.Requirements.Invalidate()`.
- Organiser steps: `stepCode` is set once (submissions refer to it); `isRequired` and
  `active` default to true; an empty description is stored as NULL.
- Sports: `code` is set once. Positions and templates must belong to an existing sport
  (404); a template's `positionId` must be a position of the same sport (422). Template
  fields: 1-50, unique lowercase keys, label 1-64, type in `sport.FieldTypes`, unit up to
  16. Changing `fields` bumps `version`; a name/active change does not.
- Audit details hold ids, versions and flags only, never policy text or config values
  (values are in `config_audit`).

## Tests
`appconfig_test.go`: bootstrap shape, public policy (unknown type / none active / found),
403 for every admin route without the permission, config list / PUT happy path with both
audit rows / 404 / 409 stale / 422 wrong type / enum key / no-op, a value-validation table
for every type and enum-like key including the prod `mock` refusal, history, policy list /
create (version = max + 1) / activate (deactivates previous) / no-op / 404, requirements
CRUD with 409 and 404, organiser steps (defaults, immutable code, partial update, 404),
sports / positions / stat templates (422 field rules, cross-sport position, version bump).

## Changing this module safely
- Adding an enum-like config key: add it to `enumKeys` (and to docs/config-system.md).
- Adding a policy type needs a migration for the ENUM and an entry in `DocTypes`.
- Keep every write in one transaction with both audit calls; publish nothing before commit.
