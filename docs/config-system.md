# Config system

Rule: **no business number or switch is hard-coded.** Fees, limits, plan contents, refund
percentages, verification steps and safety rules are rows in MySQL, edited from the admin panel,
live within ~15 seconds, and every change is written to `config_audit`.

| Where | What | Admin screen / API |
|---|---|---|
| `app_config` | key → JSON value (limits, switches, providers) | Config · `/admin/config` |
| `listing_fee_rules` | listing fee %, min/max, free-event fee, GST, per sport / event type | Fee rules · `/admin/fee-rules` |
| `premium_plans` + `premium_plan_benefits` | plans, prices, store product ids, what each plan includes | Plans · `/admin/plans` |
| `policy_documents` | versioned terms, privacy, refund policy, consent text | Policies · `/admin/policies` |
| `verification_requirements` | which role must complete which check before which action | Verification · `/admin/verification-requirements` |
| `organiser_verification_steps` | the organiser checklist | Organiser steps · `/admin/organiser-steps` |
| `sports`, `sport_positions`, `stat_templates` | sports catalogue and stat fields | Sports · `/admin/sports` |

## Reading config in Go

```go
n := d.Settings.Int(ctx, "chat.message_max_length", 2000)     // default used only if the row is missing
on := d.Settings.Bool(ctx, "event.require_admin_approval", true)
ttl := d.Settings.Seconds(ctx, "auth.otp.ttl_seconds", 300)
types := d.Settings.StringList(ctx, "media.allowed_image_types", nil)
```

- The Go default must equal the seed default (it only protects against a missing row).
- `settings.Store` caches all rows in memory for 15 s per process; the admin save calls
  `Invalidate()` on the process that served it, others catch up within the TTL.
- Rows with `is_public = 1` are sent to the app in `GET /v1/bootstrap` → `config`. Never
  mark a key public if it helps abuse (rate limits, thresholds, providers).
- Tests use `settings.NewStatic(map[string]any{...})`.

### Adding a key

1. New migration `00NN_<what>.sql` with `INSERT INTO app_config ...` (never edit a released
   migration; during the initial build `0007_seed.sql` may still be edited).
2. Read it with a typed getter and the same default.
3. Add a row to the table below.

### Value types

`int` (JSON number), `decimal` (JSON number), `bool`, `string` (JSON string), `string_list`
(JSON array of strings), `json` (any object). The admin PUT validates the value against
`value_type` and rejects mismatches with 422. Enum-like strings (e.g. `minor.dm_policy`) are
validated by the admin service against the allowed values listed in the Meaning column.

## `app_config` keys

### `auth`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `auth.otp.length` | int | `6` | yes | Number of OTP digits |
| `auth.otp.ttl_seconds` | int | `300` | yes | OTP validity in seconds |
| `auth.otp.max_attempts` | int | `5` | no | Wrong OTP attempts before the request is burnt |
| `auth.otp.resend_cooldown_seconds` | int | `30` | yes | Seconds before an OTP can be resent |
| `auth.otp.max_per_hour` | int | `5` | no | OTP requests per mobile per hour |
| `auth.otp.max_per_ip_per_hour` | int | `100` | no | OTP requests per client IP per hour (higher: mobile carriers share one IP across many users via CGNAT) |
| `auth.access_token_ttl_minutes` | int | `30` | no | Access token lifetime |
| `auth.refresh_token_ttl_days` | int | `60` | no | Refresh token lifetime |
| `auth.sms_provider` | string | `"stub"` | no | stub | msg91 |
| `auth.min_age_years` | int | `10` | yes | Minimum age to use Athlix (checked against verified DOB) |

### `verification`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `verification.provider` | string | `"mock"` | no | digilocker_direct | aggregator | mock (mock is refused when ENV=prod) |
| `verification.max_attempts_per_day` | int | `3` | no | DigiLocker attempts per user per day |
| `verification.state_ttl_seconds` | int | `600` | no | How long a DigiLocker consent link stays valid |
| `verification.name_match_required` | bool | `false` | no | Require display name to resemble the DigiLocker name |
| `verification.subject_hash_retention_days` | int | `180` | no | Days after account deletion before the identity subject hash and provider ref are cleared (job `verification.purge_subject_hash`) |

### `guardian`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `guardian.minor_age_years` | int | `18` | yes | Below this age a user is a minor |
| `guardian.consent_otp_ttl_seconds` | int | `600` | no | Parent consent OTP validity |
| `minor.dm_policy` | string | `"mutual_follow"` | yes | Who can DM a minor: mutual_follow | none | anyone |
| `minor.profile_default_visibility` | string | `"followers"` | yes | Default profile visibility for minors |
| `minor.searchable` | bool | `true` | no | Minors appear in people search (limited fields) |
| `minor.can_join_public_communities` | bool | `true` | yes | Minors may join public communities |

### `event`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `event.require_admin_approval` | bool | `true` | yes | Paid listings wait for admin approval before publishing |
| `event.show_contact` | string | `"after_registration"` | yes | When organiser contact is shown: always | after_registration |
| `event.max_active_per_organiser` | int | `20` | yes | Max draft+pending+published events per organiser |
| `event.min_hours_before_start` | int | `24` | yes | Event must start at least this many hours after publishing |
| `event.auto_create_group` | bool | `true` | yes | Create a group chat for every published event |
| `event.allow_free_events` | bool | `true` | yes | Allow events with entry fee 0 |
| `event.max_slots_limit` | int | `1024` | yes | Upper bound for max_slots |
| `event.contact_reveals_per_day` | int | `50` | no | Organiser contact reveals one user may do per day (anti-scraping) |
| `event.featured_list_size` | int | `10` | no | Max featured events returned by GET /events/featured |
| `event.max_search_radius_km` | int | `200` | yes | Largest `radiusKm` accepted by GET /events |
| `event.eligibility_default_government` | string | `"native"` | yes | Suggested eligibility type for government / official tournaments |
| `event.eligibility_default_private` | string | `"open"` | yes | Suggested eligibility type for private / open tournaments |

### `location`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `location.enabled` | bool | `true` | yes | Location identities and location-based eligibility are on (off = every event open) |
| `location.residence_validity_months` | int | `12` | yes | A verified residence identity expires after this many months |
| `location.residence_change_cooldown_days` | int | `180` | yes | Minimum days between residence changes to a different district |
| `location.native_change_allowed` | bool | `true` | yes | Allow a reviewed native change (e.g. birth certificate replacing the DigiLocker district) |
| `location.proof_retention_days` | int | `30` | yes | Days after review before the proof document is deleted (result is kept) |
| `location.expiry_reminder_days` | int | `30` | no | Remind users this many days before residence expiry |
| `location.native_proof_types` | string_list | `["masked_aadhaar","birth_certificate"]` | yes | Upload proofs accepted for native identity (DigiLocker is automatic) |
| `location.residence_proof_types` | string_list | `["rent_agreement","utility_bill","bank_statement","voter_id","employer_letter","institution_id"]` | yes | Proofs accepted for residence identity |
| `location.utility_bill_max_age_days` | int | `90` | yes | Reviewer checklist: utility bill / bank statement must be this recent |
| `location.minor_guardian_proof_allowed` | bool | `true` | yes | Minors may submit a parent/guardian address proof for residence |
| `location.flag_registrations_on_change` | bool | `true` | no | Flag (never cancel) registrations when the qualifying identity stops being valid |
| `location.rejection_reasons` | string_list | `["unclear_document","name_mismatch","address_mismatch","district_mismatch","document_too_old","unmasked_aadhaar","wrong_document_type","other"]` | no | Reason codes a reviewer can pick; `unmasked_aadhaar` deletes the file at once |
| `location.proof_view_url_ttl_seconds` | int | `120` | no | Lifetime of the signed URL a reviewer gets for a proof (capped at 300) |

### `listing`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `listing.payment_provider` | string | `"mock"` | no | razorpay | mock (mock refused when ENV=prod) |
| `listing.order_ttl_minutes` | int | `30` | yes | Unpaid listing order expiry |
| `listing.refund.before_first_registration_percent` | int | `100` | yes | Refund % if organiser cancels before anyone registers |
| `listing.refund.after_registrations_percent` | int | `0` | yes | Refund % if organiser cancels after registrations exist |
| `listing.refund.on_admin_rejection_percent` | int | `100` | yes | Refund % when admin rejects the event |

### `registration`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `registration.withdraw_until_hours` | int | `24` | yes | Players can withdraw until this many hours before start |
| `registration.waitlist_enabled` | bool | `true` | yes | Accept waitlist registrations when slots are full |

### `chat`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `chat.message_max_length` | int | `2000` | yes | Max characters in a message |
| `chat.group_max_members` | int | `500` | yes | Max members in a community or event group |
| `chat.max_communities_created_per_user` | int | `10` | yes | Communities one user can own |
| `chat.messages_per_minute` | int | `30` | no | Per-user send rate limit |
| `chat.media_enabled` | bool | `true` | yes | Allow image messages |
| `chat.dm_requires_follow` | bool | `false` | yes | Adults: only DM people who follow you |
| `chat.ws_ticket_ttl_seconds` | int | `60` | no | WebSocket ticket lifetime |

### `feed`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `feed.post_max_length` | int | `2000` | yes | Max characters in a post (capped at 2000, the column size) |
| `feed.post_max_media` | int | `4` | yes | Max images per post |
| `feed.comment_max_length` | int | `1000` | yes | Longest comment body in characters (1 to 1000, the column size) |
| `feed.page_size` | int | `20` | yes | Feed page size |

### `profile`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `profile.viewers_window_days` | int | `30` | yes | Profile viewers shown to premium users for this many days |
| `profile.max_achievements` | int | `50` | yes | Max achievements per user |

### `media`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `media.image_max_mb` | int | `5` | yes | Max image size |
| `media.document_max_mb` | int | `10` | yes | Max document (certificate, organiser proof) size |
| `media.allowed_image_types` | string_list | `["image/jpeg","image/png","image/webp"]` | yes | Allowed image MIME types |
| `media.allowed_document_types` | string_list | `["application/pdf","image/jpeg","image/png"]` | yes | Allowed document MIME types |
| `media.upload_url_ttl_seconds` | int | `300` | no | Presigned upload URL lifetime |
| `media.view_url_ttl_seconds` | int | `900` | no | Signed view URL lifetime |
| `media.pending_cleanup_hours` | int | `24` | no | Uploads never confirmed within this many hours are deleted by `media.cleanup_pending` |

### `moderation`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `moderation.auto_hide_report_threshold` | int | `5` | no | Distinct reports that auto-hide content pending review |
| `moderation.report_rate_limit_per_hour` | int | `20` | no | Reports one user may file per hour (429 above it) |

### `search`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `search.page_size` | int | `20` | yes | Search page size |

### `ads`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `ads.enabled` | bool | `false` | yes | Show sponsored placements |
| `ads.show_to_minors` | bool | `false` | no | Show sponsored placements to minors |
| `ads.feed_every_n_posts` | int | `8` | yes | Insert one ad after every N feed posts |
| `ads.max_per_request` | int | `3` | no | Most sponsored placements returned by one GET /ads (1 to 10) |

### `premium`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `premium.enabled` | bool | `false` | yes | Show premium plans in the app |
| `premium.restore_max_purchases` | int | `20` | no | Most store purchases one `POST /premium/purchases/restore` call may re-verify (1 to 50) |
| `consultation.max_open_requests_per_player` | int | `5` | yes | Pending consultation requests a player may have |

### `notification`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `notification.event_reminder_hours` | int | `24` | no | Remind registered players this many hours before start |

### `admin`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `admin.max_failed_logins` | int | `5` | no | Wrong passwords/TOTP before the admin account is locked |
| `admin.lockout_minutes` | int | `30` | no | Admin lockout duration |
| `admin.access_token_ttl_minutes` | int | `15` | no | Admin access token lifetime (role changes apply on next refresh) |
| `admin.refresh_token_ttl_hours` | int | `12` | no | Admin session length |
| `admin.login.max_per_ip` | int | `20` | no | Admin login attempts per client IP per window |
| `admin.login.window_minutes` | int | `15` | no | Window of the per-IP admin login rate limit, in minutes |
| `admin.password_min_length` | int | `12` | no | Minimum admin password length (8 to 72) on `POST /admin/me/password` |
| `admin.login_events_retention_days` | int | `365` | no | `admin_login_events` rows older than this are deleted (job `admin.purge_login_events`) |

### `app`

| Key | Type | Default | Public | Meaning |
|---|---|---|---|---|
| `app.min_version_android` | string | `"1.0.0"` | yes | Force update below this version |
| `app.min_version_ios` | string | `"1.0.0"` | yes | Force update below this version |
| `app.maintenance_mode` | bool | `false` | yes | Show maintenance screen |
| `app.support_email` | string | `"support@example.com"` | yes | Support contact shown in app |
| `app.grievance_officer` | json | `{"name":"To be appointed","email":"grievance@example.com"}` | yes | IT Rules grievance officer shown in app |

## Listing fee rules

```
gross    = entryFeePaise × maxSlots
base     = entryFeePaise == 0 ? freeEventFeePaise : clamp(round(gross × percentBp / 10000), minFeePaise, maxFeePaise)
gst      = round(base × gstBp / 10000)
total    = base + gst                                  (rounding: half up, to the paisa)
```

`maxFeePaise = NULL` means no cap. Rule selection: active rows with
`effective_from ≤ now < effective_to (or NULL)`, most specific first:
`sport_id AND event_type` > `sport_id` > `event_type` > both NULL; ties → higher `priority`,
then higher `id`. There must always be one active default rule; the admin API refuses to
deactivate the last one. The computed quote is copied onto `listing_orders`, so editing a
rule never changes an existing order.

Seed: 5 % of entry fee × slots, min ₹99, max ₹4,999, ₹99 for free events, +18 % GST.
Example: ₹500 × 16 teams = ₹8,000 → 5 % = ₹400 → + ₹72 GST = **₹472**.

## Premium benefits (closed list)

A plan's benefits are rows `(plan_id, benefit_key, value, label)`. Code checks keys through
`platform/entitlements`; the admin API refuses unknown keys. Adding a key = add it here, to
`entitlements.go`, and to the code that honours it.

| Key | Value | Effect | Checked in |
|---|---|---|---|
| `consultation_listing` | `true` | coach may offer consultations (only enforced when `verification_requirements` coach/`coach_pro_subscription`/`offer_consultation` is required) | consultation |
| `profile_boost` | `true` | ranked first in people search | search |
| `advanced_stats` | `true` | career totals on the user's stats | profile |
| `see_profile_viewers` | `true` | `GET /me/profile-viewers` (window `profile.viewers_window_days`) | profile |
| `advanced_search_filters` | `true` | `ageMin/ageMax/achievementLevel` filters | search |
| `extra_communities` | int | max communities owned = max(this, `chat.max_communities_created_per_user`) | community |
| `featured_events` | int | featured placements per calendar month (IST) | event |
| `verified_coach_badge` | `true` | `"coach_pro"` badge on the profile | profile |

If a user has several active plans with the same key, the larger value wins. Plans are sold
in the app only when `premium.enabled` is true; `store_product_id_android/ios` must match the
products created in Play Console / App Store Connect.

## Verification requirements

Rows `(role, requirement, is_required, gates)`. Checked with
`d.Requirements.Required(ctx, role, requirement, gate)`; for users with several roles use
`RequiredForAny` (any role requiring it → required).

| Requirement | Gate | Meaning when `is_required = 1` |
|---|---|---|
| `digilocker` | `app_access` | onboarding has a `verification` step; `RequireOnboarded` blocks the app until done |
| `guardian_consent_if_minor` | `app_access` | minors get the `guardian` step |
| `organiser_approval` | `create_event` | `POST /events` needs `organiser_profiles.status = approved` |
| `coach_pro_subscription` | `offer_consultation` | `PUT /me/consultation {offered:true}` needs benefit `consultation_listing` |
| (any) | `create_community` | reserved; e.g. `digilocker`/`create_community` |

If the table cannot be read, `digilocker` and `guardian_consent_if_minor` are treated as
required (fail safe); everything else as not required.

## Organiser verification steps

Rows with `step_code, title, description, input_type (text|document|checkbox), is_required,
sort_order, active`. The app renders them in order; a submission is complete when every
active required step has a value. Changing steps does not change already approved organisers.

## Policies

`policy_documents(doc_type, version, active)`: one active version per type. Activating a new
version of a `requires_acceptance` type makes it appear in `Me.pendingPolicies`; the app
blocks with an accept screen until `POST /me/policies/accept`. The listing refund policy shown
before payment is the active `listing_refund` document plus the three
`listing.refund.*` percentages.
