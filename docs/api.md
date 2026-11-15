# Athlix API contract (v1)

This file is the **contract**. The backend implements it, and the app (`app/`) and admin panel
(`admin/`) call it. If code and this file disagree, this file wins: fix the code, or change
this file and the backend together in one change.

Base URL: `https://api.<domain>/v1` (local: `http://localhost:8080/v1`).

## Conventions

### Envelope

Success: `{"data": <object|array>, "meta": {...}?}`. Error:

```json
{"error": {"code": "VALIDATION_FAILED", "message": "Some fields need attention.", "fields": {"title": "is required"}}}
```

Clients switch on `error.code`, never on `message`. `message` is safe to show to users.

| HTTP | code | When |
|---|---|---|
| 400 | `BAD_REQUEST` | malformed JSON / params |
| 401 | `UNAUTHENTICATED` | missing/expired/invalid token → app refreshes once, then signs out |
| 403 | `FORBIDDEN` | authenticated but not allowed |
| 403 | `ONBOARDING_REQUIRED` | `fields.nextStep` = `role` \| `verification` \| `guardian` \| `profile` |
| 403 | `VERIFICATION_REQUIRED` | DigiLocker needed for this action |
| 403 | `ORGANISER_NOT_APPROVED` | organiser approval needed (`verification_requirements`) |
| 403 | `PLAN_REQUIRED` | needs a premium benefit; `fields.benefit` = benefit key |
| 403 | `ACCOUNT_SUSPENDED` | user suspended |
| 403 | `NOT_ELIGIBLE` | location eligibility not met; `fields.reason` = `identity_missing` \| `identity_pending` \| `identity_expired` \| `wrong_location`, `fields.action` = `verify_native` \| `verify_residence` \| `renew_residence` \| `wait_for_review`; tournament eligibility (registration): `fields.reason` = `age_group` \| `gender_category`, `fields.action` = `none` |
| 404 | `NOT_FOUND` | does not exist **or caller may not see it** (never 403 for others' resources) |
| 409 | `CONFLICT` | duplicate / stale `version` |
| 409 | `INVALID_STATE` | action not allowed in the current status |
| 409 | `LIMIT_REACHED` | a config limit was hit |
| 422 | `VALIDATION_FAILED` | `fields` has per-field messages |
| 429 | `RATE_LIMITED` | slow down |
| 502 | `UPSTREAM_ERROR` | DigiLocker / Razorpay / store failed |
| 503 | `MAINTENANCE` | `app.maintenance_mode` is on (user routes only) |
| 500 | `INTERNAL` | bug; the message is generic |

### Types

- JSON keys are **camelCase**. Ids are JSON numbers (int64). Times are RFC 3339 UTC strings
  (`2026-11-12T09:30:00Z`). Dates are `YYYY-MM-DD`.
- Money is **integer paise** in fields ending `Paise` (`49900` = ₹499.00). Never floats.
- Percentages from config are **basis points** in fields ending `Bp` (`500` = 5%).
- Optional fields are present with `null` (not omitted) in responses.
- Request bodies: unknown fields are ignored; on `PUT` a missing field means "unchanged"
  unless stated otherwise.
- `X-Request-Id` is returned on every response; include it in bug reports.

### Auth

- App: `Authorization: Bearer <accessToken>` (user JWT). Admin: `Authorization: Bearer <adminAccessToken>` (separate key).
- Access tokens are short-lived (`auth.access_token_ttl_minutes`). On 401 call
  `POST /auth/refresh` once; refresh tokens **rotate** (always store the new one). Reusing an old
  refresh token revokes the whole session family.
- Route groups (column **Auth** below):
  - `public`: no token. `user`: token, onboarding may be unfinished. `verified`: token +
    onboarding `done` (else 403 `ONBOARDING_REQUIRED`). `admin:<perm>`: admin token + permission.
    `webhook`: provider signature.

### Pagination

Lists take `?cursor=&limit=` (default per endpoint, max 100) and return
`"meta": {"nextCursor": "..."}`. No `nextCursor` = last page. Cursors are opaque.

### Common objects

```jsonc
// SportRef
{"id": 1, "code": "cricket", "name": "Cricket"}

// UserSummary: the only shape used to show another user in lists, chat, cards.
{"id": 42, "displayName": "Asha K", "photoUrl": "https://...signed...|null",
 "primaryRole": "player|coach|organiser", "sport": SportRef|null, "position": SportRef|null,
 "city": "Pune|null", "state": "Maharashtra|null", "isVerified": true}

// TokenPair
{"accessToken": "...", "accessTokenExpiresAt": "...", "refreshToken": "...", "refreshTokenExpiresAt": "..."}

// Me: the signed-in user
{"id": 42, "displayName": "Asha K|null", "mobileMasked": "******3210",
 "primaryRole": "player|null", "roles": ["player"], "status": "active",
 "onboardingStep": "role|verification|guardian|profile|done",
 "verificationStatus": "unverified|pending|verified|failed",
 "isMinor": false, "guardianConsentStatus": "not_required|pending|granted|revoked",
 "photoUrl": null, "storeAccountToken": "uuid (pass to Play/App Store purchase, see Premium)",
 "pendingPolicies": [{"id": 3, "docType": "terms", "version": 2, "title": "Terms of Use"}],
 "createdAt": "..."}
```

Signed URLs (`photoUrl`, `imageUrl`, `bannerUrl`, `url`) expire (`media.view_url_ttl_seconds`).
Never cache them longer; re-fetch the object.

---

## Public

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/health` | public | → `{"status":"ok"}` |
| GET | `/bootstrap` | public | → Bootstrap |
| GET | `/policies/:docType` | public | → PolicyDocument (active version) |
| GET | `/sports` | public | → `[Sport]` |

```jsonc
// Bootstrap: call on app start. config = every app_config row with is_public=1, key → value.
{"config": {"auth.otp.length": 6, "chat.message_max_length": 2000, "...": "..."},
 "policies": [{"id": 1, "docType": "terms", "version": 1, "title": "Terms of Use", "requiresAcceptance": true}],
 "sports": [Sport], "minVersion": {"android": "1.0.0", "ios": "1.0.0"},
 "maintenance": false, "serverTime": "..."}

// Sport
{"id": 1, "code": "cricket", "name": "Cricket", "isTeamSport": true,
 "positions": [{"id": 1, "code": "batter", "name": "Batter", "appliesTo": "player|coach|both"}],
 "statTemplates": [{"id": 1, "positionId": null, "name": "Cricket season", "version": 1,
   "fields": [{"key": "runs", "label": "Runs", "type": "int|decimal|text|duration_ms|percent", "unit": null}]}]}

// PolicyDocument
{"id": 1, "docType": "terms|privacy|listing_refund|guardian_consent|community_guidelines|organiser_terms|entry_fee_disclaimer|location_identity_notice",
 "version": 1, "title": "...", "contentMd": "# ...", "requiresAcceptance": true, "effectiveFrom": "..."}
```

## Auth (module `auth`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| POST | `/auth/otp/request` | public | `{mobile}` → `{requestId, resendAfterSeconds, expiresInSeconds}` |
| POST | `/auth/otp/verify` | public | `{requestId, otp, deviceId?}` → `{tokens: TokenPair, user: Me, isNewUser}` |
| POST | `/auth/refresh` | public | `{refreshToken}` → `TokenPair` |
| POST | `/auth/logout` | user | `{refreshToken?}` → 204 (revokes that session family; without body: all sessions) |

- `mobile`: Indian mobile, 10 digits, optional `+91`/`91` prefix. Normalised to `+91XXXXXXXXXX`.
- Rate limits: `auth.otp.max_per_hour` per mobile and per IP; resend after
  `auth.otp.resend_cooldown_seconds` (429 `RATE_LIMITED`). After `auth.otp.max_attempts` wrong
  codes the request is burnt (422, ask for a new OTP).
- `otp/request` always returns 200 for a valid number (does not reveal if the user exists).
- New user: row created on first successful verify (`isNewUser: true`, emits `user.registered`).
- `ENV=local` only: if `DEV_FIXED_OTP` is set, that code works and no SMS is sent.

## Me, onboarding, verification (modules `user`, `verification`, `guardian`)

Onboarding steps, in order: `role` → `verification` → `guardian` (only if minor) → `profile` → `done`.
Whether a role needs DigiLocker comes from `verification_requirements` (gate `app_access`).

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/me` | user | → Me |
| DELETE | `/me` | user | → 204. Soft delete: status `deleted`, PII ciphertext wiped, tokens revoked, posts/messages hidden |
| GET | `/me/settings` | user | → Settings |
| PUT | `/me/settings` | user | Settings (partial) → Settings |
| POST | `/me/policies/accept` | user | `{policyDocumentIds: [1,2]}` → Me |
| POST | `/me/roles` | verified | `{role}` → Me (adds a secondary role; organiser then needs organiser verification) |
| POST | `/onboarding/role` | user | `{role: "player|coach|organiser", additionalRoles?: []}` → Me (step `role` only) |
| POST | `/verification/digilocker/start` | user | `{}` → `{authorizationUrl, expiresAt}` |
| GET | `/verification/digilocker/callback` | public (one-time `state`) | `?code&state` → **302** `athlix://verification/result?status=verified|minor|failed&reason=` |
| GET | `/verification/status` | user | → VerificationStatus |
| POST | `/verification/mock/complete` | user | `{outcome: "adult|minor|too_young|duplicate|failed", nativeDistrictId?: 26}` (district stands in for the Aadhaar address district) → VerificationStatus. **Only when `verification.provider`=`mock` and ENV≠prod**, else 404 |
| POST | `/onboarding/guardian/request` | user | `{guardianName, guardianMobile, relation: "mother|father|legal_guardian|other", policyDocumentId}` → `{consentId, guardianMobileMasked, expiresInSeconds, resendAfterSeconds}` |
| POST | `/onboarding/guardian/verify` | user | `{consentId, otp}` → Me |
| POST | `/me/guardian/withdraw/request` | user (minor, consent granted) | `{}` → `{consentId, guardianMobileMasked, expiresInSeconds, resendAfterSeconds}` (code goes to the guardian's mobile) |
| POST | `/me/guardian/withdraw/verify` | user | `{consentId, otp}` → Me (consent `revoked`, step back to `guardian`) |
| POST | `/onboarding/profile` | user | ProfileSetup → Me (step becomes `done`) |

```jsonc
// Settings
{"notificationPrefs": {"messages": true, "events": true, "social": true, "marketing": false},
 "privacy": {"allowDmsFrom": "everyone|followers|mutual|nobody", "showInSearch": true}}

// VerificationStatus
{"status": "unverified|pending|verified|failed", "provider": "digilocker_direct|aggregator|mock|null",
 "aadhaarLast4": "1234|null", "verifiedAt": "...|null",
 "failureReason": "too_young|already_linked|provider_error|cancelled|name_mismatch|null",
 "isMinor": false, "attemptsLeftToday": 3}

// ProfileSetup
{"displayName": "Asha K", "sportId": 1, "positionId": 2, "city": "Pune", "district": null,
 "state": "Maharashtra", "bio": null, "academyClub": null, "photoUploadId": null}
```

DigiLocker rules (see [flows/signup-digilocker.md](flows/signup-digilocker.md)):

- The **server** does the OAuth2 + PKCE code exchange; the app only opens `authorizationUrl` in
  a browser tab and receives the deep link. `state` is single-use, bound to the user, valid
  `verification.state_ttl_seconds`.
- Kept: status, provider ref, Aadhaar **last 4**, encrypted name + DOB, gender, HMAC of the
  provider subject id (one identity = one account → `already_linked`). **Thrown away:** the full
  Aadhaar number, XML, PDF, photo and the DigiLocker access token.
- Age from DOB: below `auth.min_age_years` → `failed/too_young`; below
  `guardian.minor_age_years` → `isMinor: true`, step `guardian`. `profiles.birth_year` is set from it.
- `verification.max_attempts_per_day` per user (429).

Guardian consent: the OTP goes to the **guardian's** mobile (`guardian_otp` SMS template). The
guardian mobile must differ from the user's own mobile. On success
`guardianConsentStatus: granted`, emits `guardian.consent_granted`.

Withdrawing consent: the guardian proves it with a fresh OTP to their mobile (`/me/guardian/withdraw/*`),
or an admin with `users.write` revokes it (`POST /admin/users/:id/guardian-consent/revoke`). Either way
`guardianConsentStatus: revoked`, the user is sent back to the `guardian` step, and
`guardian.consent_revoked` is emitted.

## Location identity (module `locationidentity`)

Some tournaments are only for players **from** a district/state (native) or **living in** it
(residence). A user has at most one verified and one pending identity per type. Rules engine:
`platform/eligibility` (the only implementation). Flow: [flows/location-eligibility.md](flows/location-eligibility.md).
Everything below is off when `location.enabled` is false (every event behaves as `open`).

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/geo/states` | user | → `[State]` (active, by name) |
| GET | `/geo/districts` | user | `?stateId` (required) → `[District]` (active, by name) |
| GET | `/me/location-identities` | verified | → LocationIdentities |
| POST | `/me/location-identities` | verified | LocationIdentityInput → 201 LocationIdentity (`pending`) |
| DELETE | `/me/location-identities/:id` | verified, owner | → 204. `pending` or `verified` → `withdrawn`; proof deleted at once |

```jsonc
// State / District
{"id": 23, "code": "TN", "name": "Tamil Nadu", "isUT": false}
{"id": 26, "stateId": 23, "name": "Thanjavur"}

// LocationIdentity
{"id": 5, "identityType": "native|residence", "district": District, "state": State,
 "source": "digilocker|upload", "proofType": "digilocker_aadhaar|masked_aadhaar|birth_certificate|rent_agreement|utility_bill|bank_statement|voter_id|employer_letter|institution_id",
 "proofHolder": "self|guardian", "aadhaarLast4": "1234|null",
 "status": "pending|verified|rejected|expired|superseded|revoked|withdrawn",
 "rejectionCode": "unclear_document|...|null", "rejectionReason": "...|null",
 "submittedAt": "...", "reviewedAt": "...|null", "verifiedAt": "...|null", "expiresAt": "...|null" /* residence */}

// LocationIdentities: one slot per type
{"native": IdentitySlot, "residence": IdentitySlot,
 "notice": {"policyDocumentId": 9, "version": 1, "title": "Why we ask for location proof"},
 "proofTypes": {"native": ["masked_aadhaar", "birth_certificate"], "residence": ["rent_agreement", "..."]},
 "guardianProofAllowed": true /* caller is a minor and location.minor_guardian_proof_allowed */}

// IdentitySlot
{"displayStatus": "not_submitted|pending|verified|rejected|expired",
 "current": LocationIdentity|null  /* verified, else the latest expired/rejected/revoked */,
 "pending": LocationIdentity|null,
 "canSubmit": true, "nextChangeAt": "...|null" /* residence cooldown end */,
 "expiresInDays": 40|null}

// LocationIdentityInput
{"identityType": "residence", "districtId": 4, "proofType": "rent_agreement",
 "proofUploadId": 101 /* media purpose location_proof, confirmed */, "proofHolder": "self|guardian",
 "noticePolicyDocumentId": 9}
```

`displayStatus`: `verified` if a verified, unexpired row exists; else `pending` if one is in
review; else `expired` / `rejected` from the latest row (a `revoked` row shows as `rejected`, with the
revoke reason in `rejectionReason`); else `not_submitted`.
When `location.enabled` is false, `canSubmit` is false for both slots and POST returns
409 `INVALID_STATE`; existing identities are still listed.

Rules:
- **Native from DigiLocker**: when DigiLocker verification succeeds, the Aadhaar address
  district is matched to `districts` (name or alias) and a `verified` native identity is
  created (`source: digilocker`, no document, no expiry). The address is never stored. If the
  district cannot be matched the user uploads a native proof instead.
- Native uploads: `proofType` in `location.native_proof_types`. A second native while one is
  verified needs `location.native_change_allowed` (else 409 `INVALID_STATE`); on approval the
  old one becomes `superseded`. Native never expires.
- Residence: `proofType` in `location.residence_proof_types`. Expires
  `location.residence_validity_months` after approval. Renewing for the **same** district is
  always allowed (also before expiry). Changing to a **different** district within
  `location.residence_change_cooldown_days` of the current one's `verifiedAt` → 409
  `LIMIT_REACHED` with `fields.nextChangeAt`.
- Native and residence may be the same district.
- One pending per type: a second submit → 409 `CONFLICT` (withdraw the pending one first).
- `proofHolder: guardian` only for residence, only for minors, only when
  `location.minor_guardian_proof_allowed` (else 422).
- `noticePolicyDocumentId` must be the active `location_identity_notice` (422 otherwise); it
  is recorded as a policy acceptance and on the row (`notice_version`).
- `proofUploadId` must be the caller's confirmed `location_proof` upload (else 422); it is
  claimed (attached) in the same transaction. A masked Aadhaar must show only the last 4
  digits; reviewers reject unmasked ones (`unmasked_aadhaar`) and the file is deleted at once.
- The proof file is deleted `location.proof_retention_days` after review (job); the result,
  district, proof type and dates stay. Organisers never see documents.
- `district.active` must be true (422). `stateId` is taken from the district, never the client.
- Emits `location_identity.changed` on every status change.

## Media (module `media`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| POST | `/media/uploads` | user | `{purpose, contentType, sizeBytes}` → `UploadTicket` |
| POST | `/media/uploads/:id/confirm` | user | `{}` → `{uploadId, status: "confirmed"}` |
| POST | `/admin/media/uploads` | `ads.write` | `{purpose: "post_image", contentType, sizeBytes}` → UploadTicket (ad images; owned by no user) |
| POST | `/admin/media/uploads/:id/confirm` | `ads.write` | `{}` → `{uploadId, status: "confirmed"}` (admin uploads only, else 404) |

```jsonc
// UploadTicket: then PUT the raw bytes to uploadUrl with exactly these headers.
{"uploadId": 77, "uploadUrl": "https://...", "method": "PUT",
 "headers": {"Content-Type": "image/jpeg"}, "expiresAt": "..."}
```

`purpose`: `profile_photo | achievement_certificate | event_banner | chat_image | post_image |
community_avatar | organiser_document | location_proof`. `location_proof` (PDF/JPEG/PNG, document size limit) is never returned as a signed URL to users; only reviewers get one (`location.proof_view_url_ttl_seconds`). Type/size limits: `media.*` config. The client never
sends or sees an S3 key; other endpoints take `...UploadId` and return signed URLs.

## Devices (module `notification`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| POST | `/notifications/devices` | user | `{platform: "android|ios|web", token, appVersion?}` → 204 (re-registering a token moves it to this user) |
| DELETE | `/notifications/devices` | user | `{token}` → 204 |

---

## Profiles, stats, achievements (module `profile`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/profiles/:userId` | verified | → Profile (records a profile view unless self or viewer is a minor) |
| PUT | `/me/profile` | verified | ProfileUpdate → Profile |
| GET | `/me/profile-viewers` | verified | → Page<`{user: UserSummary, viewedOn}`>, needs benefit `see_profile_viewers` (403 `PLAN_REQUIRED`) |
| GET | `/profiles/:userId/stats` | verified | → `{entries: [StatEntry], totals: {key: number}|null}` |
| POST | `/me/stats` | verified | `{sportId, templateId, season, stats: {key: value}}` → StatEntry |
| PUT | `/me/stats/:id` | verified | `{season?, stats?}` → StatEntry |
| DELETE | `/me/stats/:id` | verified | → 204 |
| GET | `/profiles/:userId/achievements` | verified | → `[Achievement]` |
| POST | `/me/achievements` | verified | AchievementInput → Achievement (max `profile.max_achievements`) |
| PUT | `/me/achievements/:id` | verified | AchievementInput (partial) → Achievement |
| DELETE | `/me/achievements/:id` | verified | → 204 |

```jsonc
// Profile
{"user": UserSummary, "isOwn": false, "restricted": false,
 "bio": "...|null", "sports": [{"sport": SportRef, "position": SportRef|null, "isPrimary": true}],
 "district": null, "academyClub": null, "birthYear": 2008 /* only when isOwn */,
 "visibility": "public|followers|private", "statsVisibility": "public|followers|private",
 "followersCount": 10, "followingCount": 4,
 "isFollowing": false, "isFollowedBy": false, "isBlockedByMe": false, "canMessage": true,
 "badges": ["coach_pro"], "coachingSinceYear": null,
 "consultation": {"offered": true} /* coaches only, else null */,
 "organiser": {"status": "approved", "orgName": "...", "orgType": "club"} /* organisers only, else null */}
```

- `restricted: true` when visibility hides the profile from the viewer: only `user`, `isOwn`,
  `restricted`, follow flags and counts are filled; the rest are null/empty.
- Blocked either way → 404. `canMessage` applies the DM rules from Chat.
- `stats.totals` (career totals per numeric key) is filled only if the **profile owner** has
  benefit `advanced_stats`; else `null`.
- `stats` values are validated against the template: unknown keys or wrong types → 422.
- `badges` come from benefits (`verified_coach_badge` → `"coach_pro"`).

```jsonc
// ProfileUpdate (all optional)
{"displayName": "...", "bio": "...", "sports": [{"sportId": 1, "positionId": 2, "isPrimary": true}],
 "city": "...", "district": "...", "state": "...", "academyClub": "...", "photoUploadId": 77,
 "visibility": "public|followers|private", "statsVisibility": "...", "coachingSinceYear": 2015}
// StatEntry
{"id": 5, "sportId": 1, "templateId": 1, "season": "2025-26", "stats": {"runs": 812}, "updatedAt": "..."}
// AchievementInput / Achievement (Achievement adds id, certificateUrl, createdAt)
{"sportId": 1, "title": "State U-16 champion", "description": null,
 "level": "school|college|club|district|state|national|international|other",
 "achievedOn": "2025-01-20", "certificateUploadId": 78}
```

Minors: `visibility: public` is refused (422) when `minor.profile_default_visibility` is not
`public`. For minors, `city` is hidden from anyone who is not a follower.

## Follow and block (module `follow`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| POST | `/users/:userId/follow` | verified | → `{following: true, followersCount}` (emits `follow.created`) |
| DELETE | `/users/:userId/follow` | verified | → `{following: false, followersCount}` |
| GET | `/users/:userId/followers` | verified | → Page<UserSummary> |
| GET | `/users/:userId/following` | verified | → Page<UserSummary> |
| POST | `/users/:userId/block` | verified | → 204 (also removes follows both ways) |
| DELETE | `/users/:userId/block` | verified | → 204 |
| GET | `/me/blocks` | verified | → Page<UserSummary> |

Cannot follow/block yourself (422). Blocked either way → 404 for follow and lists.

## Organiser verification (module `organiser`)

Requires role `organiser` (else 403 `FORBIDDEN`). The steps come from `organiser_verification_steps`.

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/organiser/verification` | verified | → OrganiserVerification |
| PUT | `/organiser/verification` | verified | `{orgName?, orgType?, steps?: [{stepId, valueText?, documentUploadId?, checked?}]}` → OrganiserVerification (only in `not_started|draft|rejected`) |
| POST | `/organiser/verification/submit` | verified | `{}` → OrganiserVerification (all required steps filled → `submitted`) |

```jsonc
// OrganiserVerification
{"status": "not_started|draft|submitted|approved|rejected", "orgName": null,
 "orgType": "individual|club|academy|school|college|association|company|other|null",
 "reviewNote": null, "submittedAt": null, "reviewedAt": null,
 "steps": [{"stepId": 1, "stepCode": "org_details", "title": "...", "description": "...",
   "inputType": "text|document|checkbox", "isRequired": true,
   "submission": {"valueText": "...", "document": {"uploadId": 9, "url": "..."}|null,
                  "checked": true, "status": "submitted|approved|rejected", "reviewNote": null}|null}]}
```

---

## Events (module `event`)

Athlix **never** collects the entry fee. `entryFeePaise` is information for players; they pay
the organiser directly using `paymentModes` / the organiser's contact.

Status machine: `draft` → (listing order) `pending_payment` → (paid) `pending_review` (if
`event.require_admin_approval`) or `published` → `completed` (job) | `cancelled` (organiser) |
`removed` (admin). Admin can `reject` from `pending_review` → `rejected` (editable, resubmit with a new order).

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| POST | `/events` | verified | EventInput → Event (`draft`) |
| PUT | `/events/:id` | verified, owner | EventInput (partial) → Event |
| GET | `/events/:id` | verified | → Event (non-published visible to owner only, else 404) |
| GET | `/events` | verified | filters → Page<EventSummary> (published only) |
| GET | `/events/eligibility/preview` | verified | `?category&type&scope&districtId&stateId&requireVerified` → EligibilityPreview (organiser form preview line) |
| GET | `/events/featured` | verified | `?sportId&state` → `[EventSummary]` (max `event.featured_list_size`) |
| GET | `/me/events` | verified | `?status` → Page<EventSummary> (my organised events) |
| GET | `/events/:id/contact` | verified | → EventContact |
| POST | `/events/:id/cancel` | verified, owner | `{reason}` → Event |
| POST | `/events/:id/feature` | verified, owner | `{}` → Event (needs benefit `featured_events` with remaining monthly count, else 403 `PLAN_REQUIRED` / 409 `LIMIT_REACHED`) |

Creating events needs role `organiser`, and, when `verification_requirements` says so,
`organiser_profiles.status = approved` (403 `ORGANISER_NOT_APPROVED`). Max
`event.max_active_per_organiser` events in draft/pending/published (409 `LIMIT_REACHED`).

```jsonc
// EventInput
{"sportId": 1, "title": "Pune U-16 Cricket Cup", "description": "...",
 "eventType": "tournament|trial|camp|match|other", "participantType": "individual|team",
 "teamSizeMin": 11, "teamSizeMax": 15,
 "competitionLevel": "open|school|college|club|district|state|national|other",
 "genderCategory": "open|male|female|mixed", "ageMin": null, "ageMax": 16,
 "venueName": "...", "addressLine": "...", "city": "Pune", "district": null, "state": "Maharashtra",
 "pincode": "411001", "latitude": null, "longitude": null,
 "startsAt": "...", "endsAt": "...", "registrationDeadline": "...",
 "entryFeePaise": 50000, "paymentModes": ["cash", "upi", "bank_transfer", "online_link"],
 "paymentInstructions": "UPI to the number below, note your team name",
 "maxSlots": 16, "rules": "...", "prizeInfo": "...",
 "contactName": "Ravi", "contactPhone": "9876543210", "bannerUploadId": 80,
 "category": "government|private",
 "eligibility": {"type": "open|native|residence|either", "scope": "district|state|null",
                 "districtId": 26|null, "stateId": 23|null, "requireVerified": true}}

// EventSummary
{"id": 9, "title": "...", "sport": SportRef, "eventType": "tournament", "participantType": "team",
 "competitionLevel": "open", "genderCategory": "open", "ageMin": null, "ageMax": 16,
 "venueName": "...", "city": "Pune", "district": null, "state": "Maharashtra",
 "startsAt": "...", "endsAt": "...", "registrationDeadline": "...",
 "entryFeePaise": 50000, "maxSlots": 16, "registrationsCount": 3, "slotsLeft": 13,
 "bannerUrl": "...|null", "status": "published", "isFeatured": false, "organiser": UserSummary,
 "category": "government|private", "eligibility": EligibilityRule, "myEligibility": MyEligibility,
 "distanceKm": 12.4|null /* only when lat/lng were sent */}

// EligibilityRule (as stored, with names)
{"type": "native", "scope": "district|null", "district": District|null, "state": State|null,
 "requireVerified": true, "summary": "Only players with a verified Thanjavur native identity can register."}

// MyEligibility (for the caller)
{"eligible": true, "via": "open|native|residence|null", "pending": false,
 "tag": "Eligible via Native: Thanjavur|null",
 "reason": "identity_missing|identity_pending|identity_expired|wrong_location|null",
 "action": "none|verify_native|verify_residence|renew_residence|wait_for_review"}

// EligibilityPreview
{"valid": true, "fields": {} /* per-field errors as in VALIDATION_FAILED */,
 "summary": "Only players with a verified Thanjavur native identity can register.",
 "suggestedType": "native" /* event.eligibility_default_<category> */}

// Event = EventSummary + these
{"description": "...", "teamSizeMin": 11, "teamSizeMax": 15, "addressLine": "...", "pincode": "...",
 "latitude": null, "longitude": null, "paymentModes": ["upi"], "paymentInstructions": "...",
 "rules": "...", "prizeInfo": "...", "contactName": "Ravi", "contactPhoneMasked": "******3210",
 "rejectionReason": null, "cancelReason": null, "publishedAt": "...|null",
 "isOwner": false, "myRegistration": Registration|null, "hasGroupChat": true,
 "listing": {"orderId": 12, "status": "paid", "totalPaise": 11682}|null /* owner only */}

// EventContact (full number, so players can call/WhatsApp the organiser)
{"contactName": "Ravi", "contactPhone": "+919876543210", "paymentModes": ["upi"], "paymentInstructions": "..."}
```

- Validation when submitting for listing: `startsAt ≥ now + event.min_hours_before_start`,
  `registrationDeadline ≤ startsAt`, `endsAt ≥ startsAt`, `1 ≤ maxSlots ≤ event.max_slots_limit`,
  `entryFeePaise = 0` only if `event.allow_free_events`, team sizes for team events.
- Editing: `draft`, `rejected` → anything; `pending_payment` → anything (cancels the open
  order); `pending_review` → nothing (409); `published` → only `description, rules, prizeInfo,
  paymentInstructions, bannerUploadId, contactName, contactPhone, venueName, addressLine`
  (fee inputs `entryFeePaise, maxSlots` and dates are locked; emits `event.updated`).
- `GET /events` filters: `q, sportId, eventType, competitionLevel, genderCategory, state,
  district, city, fromDate, toDate, maxEntryFeePaise, eligibleForMe` (age fits my birth year),
  `includeClosed` (default false = deadline in the future), `category`, `showAll`,
  `lat, lng, radiusKm`, `sort=startsAt|distance`. Sorted `startsAt` ascending by default.
- **Location eligibility in the feed** (`GET /events`, `/events/featured`, search): by default
  only events the caller is eligible for are returned (open events + events their identities
  match, SQL filter from `platform/eligibility`). `showAll=true` also returns the others with
  `myEligibility.eligible = false` so the app shows them greyed with "Verify now".
- **Distance**: `lat, lng` (the device location, sent per request, never stored) plus
  `radiusKm` (1..`event.max_search_radius_km`, else 422) keep events whose venue coordinates
  are within the radius (haversine on `latitude/longitude`; events without coordinates are
  excluded when a radius is given). `distanceKm` is rounded to 0.1. `sort=distance` needs `lat/lng`.
- **Eligibility input**: `category` defaults to `private`; `eligibility` defaults to
  `{type: open}`. Validation (422): open → no scope/ids; otherwise `scope` required, district
  scope → active `districtId`, state scope → `stateId`, and `stateId` is derived from the
  district for district scope. `requireVerified` defaults to `true`. The app pre-selects
  `event.eligibility_default_government` / `_private` for the chosen category.
  `category` and `eligibility` are locked once the event is `published`.
- `GET /events/:id` returns `myEligibility`; for non-owners who are not eligible
  `paymentInstructions` is `null` and `GET /events/:id/contact` returns 403 `NOT_ELIGIBLE`
  (the eligibility check is repeated right before payment details are revealed).
- `GET /events/:id/contact`: allowed for the owner; for others when `event.show_contact` is
  `always`, or `after_registration` and the caller has a registration (registered/waitlisted).
  Else 403 `FORBIDDEN`. Limited by `event.contact_reveals_per_day` (429).
- Cancel: allowed in `draft|pending_payment|pending_review|published`. Listing refund % =
  `listing.refund.before_first_registration_percent` if no registrations else
  `listing.refund.after_registrations_percent`. Emits `event.cancelled` (+ `listing.refunded`).

## Listing fee (module `listingpayment`)

Listing fee = `clamp(percentBp/10000 × entryFeePaise × maxSlots, minFeePaise, maxFeePaise)`, or
`freeEventFeePaise` when the entry fee is 0, **plus GST** (`gstBp`). The matching rule is the
most specific active `listing_fee_rules` row: (sport+type) > sport > type > default; ties by
`priority` DESC. Rounding: half-up to the paisa at each step. The quote is **snapshotted** on the order.

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/listing/quote` | verified | `?sportId&eventType&entryFeePaise&maxSlots` → ListingQuote (calculator before creating) |
| POST | `/events/:id/listing/quote` | verified, owner | `{}` → ListingQuote |
| POST | `/events/:id/listing/order` | verified, owner | `{}` → ListingOrder (event → `pending_payment`; total 0 → paid at once) |
| GET | `/events/:id/listing/order` | verified, owner | → ListingOrder (newest order for the event; 404 when none) |
| POST | `/events/:id/listing/confirm` | verified, owner | `{orderId, providerPaymentId, providerSignature}` → ListingOrder |
| POST | `/listing/mock/pay` | verified, owner | `{orderId}` → ListingOrder. **Only when `listing.payment_provider`=`mock` and ENV≠prod** |
| POST | `/webhooks/razorpay` | webhook (`X-Razorpay-Signature`) | Razorpay event → 200 |

```jsonc
// ListingQuote
{"feeRuleId": 1, "ruleName": "Default", "entryFeePaise": 50000, "maxSlots": 16,
 "grossEntryPaise": 800000, "percentBp": 500, "minFeePaise": 9900, "maxFeePaise": 499900,
 "isFreeEvent": false, "baseFeePaise": 40000, "gstBp": 1800, "gstPaise": 7200, "totalPaise": 47200,
 "refundPolicy": {"beforeFirstRegistrationPercent": 100, "afterRegistrationsPercent": 0, "onAdminRejectionPercent": 100},
 "refundPolicyDocumentId": 3}

// ListingOrder
{"orderId": 12, "eventId": 9, "status": "created|paid|failed|expired|refunded|partially_refunded",
 "quote": ListingQuote, "provider": "razorpay|mock|none", "providerOrderId": "order_ABC|null",
 "providerKeyId": "rzp_live_xxx|null" /* public key id for checkout, never the secret */,
 "refundPaise": 0, "expiresAt": "...", "paidAt": null, "eventStatus": "pending_payment"}
```

- Only one open (`created`) order per event; a new order request returns the open one if the
  quote is unchanged, else expires it and makes a new one.
- `confirm` verifies `HMAC_SHA256(orderId|paymentId, keySecret)`; the webhook is the backup path.
  Both are idempotent. On paid → emits `listing.paid`; event goes to `pending_review` (emits
  `event.submitted`) or `published` (emits `event.published`).
- Unpaid orders expire after `listing.order_ttl_minutes` (job); the event returns to `draft`.
- Webhook events handled: `payment.captured`, `order.paid`, `payment.failed`, `refund.processed`.
  Deduped on `payment_webhook_events(provider, provider_event_id)`; payloads are not stored.

## Registrations (module `registration`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| POST | `/events/:id/registrations` | verified | `{teamName?, teamMembers?: [{name, role?}]}` → Registration |
| DELETE | `/events/:id/registrations/me` | verified | → 204 |
| GET | `/me/registrations` | verified | `?status` → Page<Registration> (each with `event`) |
| GET | `/events/:id/registrations` | verified, event owner | `?status` → Page<RegistrationForOrganiser> |
| PATCH | `/events/:id/registrations/:regId` | verified, event owner | `{paymentStatus?, paymentNote?, status?: "rejected"}` → RegistrationForOrganiser |

```jsonc
// Registration
{"id": 31, "eventId": 9, "status": "registered|waitlisted|withdrawn|rejected",
 "paymentStatus": "unpaid|paid_online|paid_offline|waived", "paymentNote": null,
 "teamName": null, "teamMembers": [], "createdAt": "...", "event": EventSummary|null,
 "qualifiedVia": "open|native|residence", "qualifiedDistrict": District|null,
 "eligibilityFlag": "none|identity_expired|identity_rejected|identity_revoked|identity_changed",
 "eligibilityFlaggedAt": "...|null"}
// RegistrationForOrganiser = Registration (without event) + {"user": UserSummary,
//   "qualifiedLabel": "Native: Thanjavur (verified)|Residence: Coimbatore (in review)|Open"}
// GET /events/:id/registrations also takes ?flagged=true. Only the registrant is checked;
// team members are names only and are not verified.
```

- **Location eligibility** is checked on the server inside the registration transaction
  (`platform/eligibility`, identities read in the same tx): not eligible → 403 `NOT_ELIGIBLE`.
  The qualifying identity is stored (`qualifiedVia`, `qualifiedDistrict`). Waitlist promotion
  re-checks; a waitlisted player who is no longer eligible is skipped and flagged.
- When a qualifying identity later stops being valid (expired, revoked, withdrawn, superseded,
  or a pending one used under `requireVerified: false` is rejected), every `registered` /
  `waitlisted` registration on events not yet started is re-evaluated: still eligible via
  another identity → `qualified*` updated silently; otherwise `eligibilityFlag` is set and
  `registration.eligibility_flagged` is emitted. **Registrations are never cancelled
  automatically**; the organiser decides (reject via PATCH). Off when
  `location.flag_registrations_on_change` is false.
- Rules: event `published`, deadline in the future, caller is not the organiser, age fits
  `ageMin/ageMax` (from verified birth year; unknown → allowed), `genderCategory` male/female
  checked against verified gender. Full → `waitlisted` if `registration.waitlist_enabled`, else
  409 `LIMIT_REACHED`. A second registration → 409 `CONFLICT`. Max 30 team members.
- Withdraw until `registration.withdraw_until_hours` before start (409 `INVALID_STATE`). The
  first waitlisted player is promoted (emits `registration.promoted`).
- Emits `registration.created`, `registration.withdrawn`, `registration.payment_marked`.
- The organiser marks payment; Athlix only records it. The app shows the
  `entry_fee_disclaimer` policy on the registration screen.

## Premium (module `subscription`)

Plans, prices and benefits are DB config. In-app purchase goes through Google Play Billing /
Apple IAP; the server verifies with the store and never trusts the client.

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/premium/plans` | verified | `?audience` → `[Plan]` (`[]` when `premium.enabled` is false) |
| GET | `/me/subscription` | verified | → `{active: Subscription|null, benefits: {key: value}}` |
| POST | `/premium/purchases/verify` | verified | `{store: "google_play|app_store", productId, purchaseToken?, transactionId?}` → Subscription |
| POST | `/premium/purchases/restore` | verified | `{purchases: [same shape as verify]}` → `{restored, notRestored, subscription: {active, benefits}}` (each one re-verified with the store) |
| POST | `/premium/mock/activate` | verified | `{planId}` → Subscription. **ENV≠prod only** |
| POST | `/webhooks/google-play` | webhook (Pub/Sub OIDC JWT) | RTDN → 200 |
| POST | `/webhooks/app-store` | webhook (signed JWS) | Server Notification v2 → 200 |

```jsonc
// Plan
{"id": 2, "code": "coach_pro_monthly", "audience": "player|coach|organiser", "name": "Coach Pro",
 "description": "...", "pricePaise": 29900, "billingPeriod": "monthly|quarterly|yearly",
 "storeProductIdAndroid": "...|null", "storeProductIdIos": "...|null",
 "benefits": [{"key": "consultation_listing", "value": true, "label": "Offer paid consultations"}]}
// Subscription
{"id": 4, "plan": Plan, "store": "google_play|app_store|manual", "status": "active|grace|expired|cancelled|refunded",
 "autoRenew": true, "startedAt": "...", "expiresAt": "..."}
```

- The app passes `Me.storeAccountToken` as Play `obfuscatedAccountId` / Apple `appAccountToken`.
  The server checks that the store purchase carries the same token (else 404), so a purchase
  cannot be claimed by another account. Purchase tokens are stored only as a SHA-256 hash.
- Emits `subscription.activated` / `subscription.expired`.

## Coach consultations (module `consultation`)

Consultation fees are paid **directly to the coach**; Athlix only lists the offer. Whether the
coach needs a plan with `consultation_listing` comes from `verification_requirements`
(`coach_pro_subscription` / `offer_consultation`).

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/coaches/:userId/consultation` | verified | → `{coach: UserSummary, offered, description, feeNote, modes: ["online","in_person"], canRequest}` (404 if not offered, unless self) |
| GET | `/me/consultation` | verified (coach) | → same shape (own offer, live or not; `offered: false` when nothing saved) |
| PUT | `/me/consultation` | verified (coach) | `{offered, description?, feeNote?, modes?}` → same shape |
| POST | `/coaches/:userId/consultation-requests` | verified | `{message, preferredMode}` → ConsultationRequest |
| GET | `/me/consultation-requests` | verified | `?as=coach|player&status` → Page<ConsultationRequest> |
| POST | `/consultation-requests/:id/respond` | verified (that coach) | `{accept: true, note?}` → ConsultationRequest |
| POST | `/consultation-requests/:id/cancel` | verified (that player) | `{}` → ConsultationRequest (pending only) |
| POST | `/consultation-requests/:id/complete` | verified (that coach) | `{}` → ConsultationRequest (accepted only) |

```jsonc
// ConsultationRequest
{"id": 3, "coach": UserSummary, "player": UserSummary, "message": "...", "preferredMode": "online|in_person",
 "status": "pending|accepted|declined|cancelled|completed", "createdAt": "...", "updatedAt": "..."}
```

Max `consultation.max_open_requests_per_player` pending requests (409). Accepted → both can
open a DM with `POST /chat/direct` (adults only; minors still follow `minor.dm_policy`).

## Reports (module `moderation`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| POST | `/reports` | verified | `{targetType: "user|event|post|comment|message|conversation", targetId, reason, details?}` → 201 `{id}` |

`reason`: `spam|fake_profile|harassment|scam|inappropriate|underage_safety|payment_issue|other`.
Reporting the same target twice returns 200 with the existing id. The reporter must be able to
see the target (else 404). When distinct open reports reach
`moderation.auto_hide_report_threshold`, the target is hidden pending review.

## Ads (module `ads`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/ads` | verified | `?placement=home_feed|events_list|search|event_detail&sportId&state` → `[Ad]` |
| POST | `/ads/:id/impression` | verified | → 204 |
| POST | `/ads/:id/click` | verified | → 204 |

```jsonc
{"id": 1, "advertiserName": "...", "title": "...", "body": "...|null", "imageUrl": "...|null",
 "targetUrl": "https://...", "placement": "home_feed", "label": "Sponsored"}
```

Returns `[]` when `ads.enabled` is false, or the user is a minor and `ads.show_to_minors` is
false. Targeting is **contextual only** (placement, sport, state); no personal tracking. The
app inserts one ad after every `ads.feed_every_n_posts` feed posts.

## Search (module `search`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/search/people` | verified | filters → Page<PersonResult> |
| GET | `/search` | verified | `?q` → `{people: [UserSummary], events: [EventSummary], communities: [Community]}` (max 5 each) |

`/search/people` filters: `q, role, sportId, positionId, state, district, city`; **advanced**
(need benefit `advanced_search_filters`, else 403 `PLAN_REQUIRED`): `ageMin, ageMax,
achievementLevel`. `PersonResult = UserSummary + {"bio": "...|null", "followersCount": 3}`.
Users with `profile_boost` rank first. Excluded: blocked either way, not `done` onboarding,
suspended/deleted, `privacy.showInSearch=false`. Minors only if `minor.searchable`, and then
without `photoUrl`, `city` and `bio` unless the viewer is a mutual follow.

---

## Chat (module `chat`)

One conversation table for three kinds: `direct` (2 people), `community` (user-created group,
see Communities) and `event` (auto group for a published event).

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| POST | `/chat/ws-ticket` | verified | → `{ticket, url, expiresAt}` |
| GET | `/chat/ws` | ticket (`?ticket=`) | WebSocket upgrade (**local dev only**; in AWS `url` points at API Gateway WebSocket) |
| GET | `/chat/conversations` | verified | → Page<Conversation> (latest activity first) |
| GET | `/chat/unread-count` | verified | → `{conversations, messages}` (chat tab badge; muted conversations not counted) |
| POST | `/chat/direct` | verified | `{userId}` → Conversation (get or create) |
| GET | `/chat/conversations/:id` | verified, member | → Conversation |
| GET | `/chat/conversations/:id/messages` | verified, member | `?before=<messageId>&limit` → Page<Message> (newest first) |
| POST | `/chat/conversations/:id/messages` | verified, member | `{clientMsgId, kind: "text|image", body?, mediaUploadId?, replyToId?}` → 201 Message |
| DELETE | `/chat/messages/:id` | verified, sender or group owner/admin | → 204 |
| POST | `/chat/conversations/:id/read` | verified, member | `{messageId}` → 204 |
| POST | `/chat/conversations/:id/mute` | verified, member | `{mutedUntil: "...|null"}` → 204 |
| POST | `/chat/conversations/:id/leave` | verified, member | → 204 (not for `direct`; owner must delete or hand over first) |
| GET | `/chat/conversations/:id/members` | verified, member | → Page<`{user: UserSummary, role: "owner|admin|member", joinedAt}`> |
| GET | `/events/:id/group` | verified | → Conversation (organiser + registered players; else 404) |

```jsonc
// Conversation
{"id": 50, "type": "direct|community|event", "title": "...|null", "avatarUrl": "...|null",
 "otherUser": UserSummary|null /* direct only */, "eventId": null, "memberCount": 2,
 "myRole": "owner|admin|member", "muted": false, "unreadCount": 3,
 "lastMessage": Message|null, "status": "active|archived|removed", "updatedAt": "..."}
// Message
{"id": 900, "conversationId": 50, "sender": UserSummary|null /* null for system */,
 "kind": "text|image|system", "body": "...|null", "imageUrl": "...|null",
 "replyTo": {"id": 899, "senderName": "Asha K", "bodyPreview": "first 80 chars"}|null,
 "clientMsgId": "uuid|null", "status": "visible|deleted|hidden", "createdAt": "..."}
```

**DM rules** (`POST /chat/direct`, also `Profile.canMessage`), all must pass:
1. Not blocked either way; recipient onboarding `done` and active.
2. Recipient's `privacy.allowDmsFrom`: `everyone` | `followers` (recipient follows sender) |
   `mutual` | `nobody`. If `chat.dm_requires_follow`, at least `followers`.
3. If **either** person is a minor, `minor.dm_policy`: `none` → refused; `mutual_follow` →
   they must follow each other; `anyone` → only rules 1–2.
4. Exception for adults: an `accepted` consultation between them passes rule 2.
Failure → 403 `FORBIDDEN` "You can't message this person."

Sending: idempotent on `clientMsgId` (same id → the same message back). Limits:
`chat.message_max_length`, `chat.messages_per_minute` (429), `chat.media_enabled` (image needs
a `chat_image` upload). A message is saved, then pushed to online members over WebSocket;
`message.created` makes the notification module push to offline, unmuted members.

### WebSocket protocol

Connect to `url?ticket=<ticket>` (ticket single-use, valid `chat.ws_ticket_ttl_seconds`).
Frames are JSON text.

Client → server (`action` selects the route on API Gateway):

```jsonc
{"action": "sendMessage", "data": {"conversationId": 50, "clientMsgId": "uuid", "kind": "text", "body": "hi", "mediaUploadId": null, "replyToId": null}}
{"action": "typing",      "data": {"conversationId": 50}}
{"action": "markRead",    "data": {"conversationId": 50, "messageId": 900}}
{"action": "ping"}
```

Server → client:

```jsonc
{"type": "ack",             "data": {"clientMsgId": "uuid", "message": Message}}
{"type": "message.new",     "data": Message}
{"type": "message.deleted", "data": {"conversationId": 50, "messageId": 900}}
{"type": "typing",          "data": {"conversationId": 50, "userId": 42}}
{"type": "read",            "data": {"conversationId": 50, "userId": 42, "messageId": 900}}
{"type": "error",           "data": {"clientMsgId": "uuid|null", "code": "RATE_LIMITED", "message": "..."}}
{"type": "pong"}
```

The app must also work without the socket (REST send + poll `GET .../messages` on resume).
API Gateway closes idle sockets after 10 minutes: send `ping` every 5 minutes, reconnect with
a new ticket on close.

## Communities (module `community`)

A community **is** a conversation of type `community`: `Community.id` = its conversation id.

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| POST | `/communities` | verified | CommunityInput → Community |
| GET | `/communities` | verified | `?q&sportId&state&cursor` → Page<Community> (public, active) |
| GET | `/me/communities` | verified | → Page<Community> |
| GET | `/communities/:id` | verified | → Community (private: members/invited only, else 404) |
| PUT | `/communities/:id` | verified, owner/admin | CommunityInput (partial) → Community |
| DELETE | `/communities/:id` | verified, owner | → 204 (archived) |
| POST | `/communities/:id/join` | verified | `{}` → `{myStatus: "active|pending"}` |
| GET | `/communities/:id/join-requests` | verified, owner/admin | → Page<`{user: UserSummary, requestedAt}`> |
| POST | `/communities/:id/join-requests/:userId/approve` | verified, owner/admin | → 204 |
| POST | `/communities/:id/join-requests/:userId/reject` | verified, owner/admin | → 204 |
| POST | `/communities/:id/invites` | verified, owner/admin | `{userIds: [1,2]}` → `{invited: [1], skipped: [2]}` |
| POST | `/communities/:id/invites/accept` | verified, invited | `{}` → Community |
| POST | `/communities/:id/members/:userId/remove` | verified, owner/admin | → 204 |
| POST | `/communities/:id/members/:userId/ban` | verified, owner/admin | → 204 |
| PUT | `/communities/:id/members/:userId/role` | verified, owner | `{role: "admin|member"}` → 204 |
| POST | `/communities/:id/transfer-ownership` | verified, owner | `{userId}` → 204 (new owner must be an active admin under their own community limit; caller becomes admin; emits `community.ownership_transferred`) |

```jsonc
// CommunityInput
{"title": "Pune Sunday Football", "description": "...", "avatarUploadId": null, "sportId": 2,
 "city": "Pune", "state": "Maharashtra", "visibility": "public|private", "joinPolicy": "open|approval|invite"}
// Community
{"id": 51, "title": "...", "description": "...", "avatarUrl": null, "sport": SportRef|null,
 "city": "...", "state": "...", "visibility": "public", "joinPolicy": "open", "memberCount": 12,
 "owner": UserSummary, "myStatus": "none|active|pending|invited|banned", "myRole": "owner|admin|member|null",
 "createdAt": "..."}
```

- Anyone verified can create, up to `chat.max_communities_created_per_user` (or the larger
  `extra_communities` benefit value). Max `chat.group_max_members` members (409).
- Minors: joining public communities only if `minor.can_join_public_communities`. Inviting a
  minor follows the same `minor.dm_policy` check as a DM (else listed in `skipped`).
- Emits `community.join_requested`, `community.join_approved`.

## Feed (module `feed`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/feed` | verified | `?mode=following|discover&cursor` → Page<Post> |
| GET | `/users/:userId/posts` | verified | → Page<Post> |
| POST | `/posts` | verified | `{body?, mediaUploadIds?: [], sportId?, eventId?, visibility?: "public|followers"}` → Post |
| GET | `/posts/:id` | verified | → Post |
| DELETE | `/posts/:id` | verified, author | → 204 |
| POST | `/posts/:id/like` | verified | → `{liked: true, likeCount}` (emits `post.liked`) |
| DELETE | `/posts/:id/like` | verified | → `{liked: false, likeCount}` |
| GET | `/posts/:id/comments` | verified | → Page<Comment> (oldest first) |
| POST | `/posts/:id/comments` | verified | `{body}` → Comment (emits `post.commented`) |
| DELETE | `/comments/:id` | verified, author or post author | → 204 |

```jsonc
// Post
{"id": 70, "author": UserSummary, "body": "...|null", "images": ["https://..."], "sport": SportRef|null,
 "eventId": null, "visibility": "public|followers", "likeCount": 2, "commentCount": 1,
 "likedByMe": false, "isOwn": false, "createdAt": "..."}
// Comment
{"id": 5, "postId": 70, "author": UserSummary, "body": "...", "isOwn": false, "createdAt": "..."}
```

`following` = posts by people I follow + mine; `discover` = recent public posts (my sports
first). Limits `feed.post_max_length`, `feed.post_max_media`, page size `feed.page_size`.
Minors' posts default to `followers` when `minor.profile_default_visibility` is not `public`.
Blocked either way → hidden / 404.

## Notifications (module `notification`)

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/notifications` | verified | → Page<Notification>, `meta.unreadCount` |
| GET | `/notifications/unread-count` | verified | → `{unreadCount}` (bell badge) |
| POST | `/notifications/read` | verified | `{ids?: [1,2], all?: true}` → 204 |

```jsonc
{"id": 1, "type": "event.published", "title": "Your event is live", "body": "...",
 "data": {"eventId": 9} /* deep-link keys: eventId, conversationId, userId, postId, communityId, consultationRequestId */,
 "readAt": null, "createdAt": "..."}
```

Every domain event maps to in-app notifications (+ FCM push if the user's
`notificationPrefs` category is on). Push text contains display names only, never mobile
numbers or other PII. Mapping table: [architecture.md#notifications](architecture.md#notifications).

---

## Admin panel

All under `/v1/admin`. Every admin route except login/refresh needs an admin token and the
permission shown. Every change is written to `admin_audit_log`; every config change also to
`config_audit`. Permissions are listed in [security.md](security.md#admin-permissions).
Admin lists use the same Page/cursor rules. Admin responses may show masked PII only
(`mobileMasked`, `aadhaarLast4`), never full numbers.

### Admin auth and admins (module `admin`)

| Method | Path | Perm | Body → Response |
|---|---|---|---|
| POST | `/admin/auth/login` | public | `{email, password, totpCode}` → `{tokens: TokenPair, admin: AdminMe}` |
| POST | `/admin/auth/refresh` | public | `{refreshToken}` → TokenPair |
| POST | `/admin/auth/logout` | any | `{refreshToken?}` → 204 |
| GET | `/admin/me` | any | → AdminMe |
| POST | `/admin/me/password` | any | `{currentPassword, newPassword, totpCode}` → `{tokens: TokenPair}` (every other session revoked; wrong password → 422, not 401) |
| GET | `/admin/dashboard` | `dashboard.read` | → Dashboard |
| GET | `/admin/admins` | `admins.manage` | → `[AdminUser]` |
| POST | `/admin/admins` | `admins.manage` | `{email, name, roleId}` → `{admin: AdminUser, temporaryPassword, totpUri}` (shown once) |
| PUT | `/admin/admins/:id` | `admins.manage` | `{name?, roleId?, status?: "active|disabled"}` → AdminUser (cannot disable yourself) |
| POST | `/admin/admins/:id/reset-credentials` | `admins.manage` | → `{temporaryPassword, totpUri}` |
| GET | `/admin/roles` | `admins.manage` | → `[{id, name, description, permissions}]` |
| POST | `/admin/roles` | `admins.manage` | `{name, description, permissions}` → role |
| PUT | `/admin/roles/:id` | `admins.manage` | `{description?, permissions?}` → role |
| GET | `/admin/audit-log` | `audit.read` | `?adminId&action&targetType&targetId` → Page<`{id, admin: {id, name}, action, targetType, targetId, details, createdAt}`> |
| GET | `/admin/login-events` | `audit.read` | `?adminId&email&outcome=success|failure` → Page<`{id, admin: {id, name}|null, outcome, reason, ip, userAgent, createdAt}`> (`reason`: `ok`, `unknown_email`, `disabled`, `locked`, `bad_password`, `bad_totp`, `totp_replayed`, `totp_not_set`; the email filter matches a stored hash) |

```jsonc
// AdminMe / AdminUser
{"id": 1, "email": "ops@...", "name": "Ops", "role": {"id": 2, "name": "operations"},
 "permissions": ["users.read"], "status": "active", "lastLoginAt": "...|null"}
// Dashboard
{"users": {"total": 0, "verified": 0, "minors": 0, "newLast7Days": 0},
 "events": {"published": 0, "pendingReview": 0, "upcoming": 0},
 "organisers": {"pendingReview": 0}, "reports": {"open": 0},
 "revenue": {"listingPaidPaiseLast30Days": 0, "listingRefundedPaiseLast30Days": 0, "activeSubscriptions": 0},
 "communities": {"active": 0}}
```

Login: bcrypt password + TOTP (RFC 6238) both required. `admin.max_failed_logins` failures
lock the account for `admin.lockout_minutes`. Errors never say which factor failed. Admin
permissions are copied into the access token; role changes apply on the next refresh
(`admin.access_token_ttl_minutes`). The first super admin is created with
`go run ./cmd/admincli create-admin` (see backend/README.md).

### Config (module `appconfig`)

| Method | Path | Perm | Body → Response |
|---|---|---|---|
| GET | `/admin/config` | `config.read` | `?category` → `[ConfigEntry]` |
| PUT | `/admin/config/:key` | `config.write` | `{value, version}` → ConfigEntry (409 if `version` is stale; 422 if value does not match `valueType`) |
| GET | `/admin/config/:key/history` | `config.read` | → `[{oldValue, newValue, changedBy: {id, name}, changedAt}]` |
| GET | `/admin/policies` | `config.read` | `?docType` → `[PolicyDocument + {active, createdAt}]` |
| POST | `/admin/policies` | `policies.write` | `{docType, title, contentMd, requiresAcceptance}` → new inactive version |
| POST | `/admin/policies/:id/activate` | `policies.write` | → PolicyDocument (the previous active version of that type is deactivated) |
| GET | `/admin/verification-requirements` | `config.read` | → `[{id, role, requirement, isRequired, gates}]` |
| POST | `/admin/verification-requirements` | `config.write` | `{role, requirement, isRequired, gates}` → row |
| PUT | `/admin/verification-requirements/:id` | `config.write` | `{isRequired}` → row |
| GET | `/admin/organiser-steps` | `config.read` | → `[{id, stepCode, title, description, inputType, isRequired, sortOrder, active}]` |
| POST | `/admin/organiser-steps` | `config.write` | step → step |
| PUT | `/admin/organiser-steps/:id` | `config.write` | step (partial) → step |
| GET | `/admin/sports` | `config.read` | → `[Sport + {active, sortOrder}]` (including inactive) |
| POST | `/admin/sports` | `sports.write` | `{code, name, isTeamSport, sortOrder}` → Sport |
| PUT | `/admin/sports/:id` | `sports.write` | `{name?, isTeamSport?, sortOrder?, active?}` → Sport |
| POST | `/admin/sports/:id/positions` | `sports.write` | `{code, name, appliesTo, sortOrder}` → position |
| PUT | `/admin/positions/:id` | `sports.write` | `{name?, appliesTo?, sortOrder?, active?}` → position |
| POST | `/admin/sports/:id/stat-templates` | `sports.write` | `{positionId?, name, fields}` → template |
| PUT | `/admin/stat-templates/:id` | `sports.write` | `{name?, fields?, active?}` → template (changing `fields` bumps `version`) |

```jsonc
// ConfigEntry
{"key": "listing.order_ttl_minutes", "value": 30, "valueType": "int|decimal|bool|string|string_list|json",
 "category": "listing", "description": "...", "isPublic": true, "version": 3, "updatedAt": "..."}
```

Saved config is live within ~15 seconds on every server (in-process cache TTL); no deploy.

### Users and organisers (modules `user`, `organiser`)

| Method | Path | Perm | Body → Response |
|---|---|---|---|
| GET | `/admin/users` | `users.read` | `?q&role&status&verificationStatus&isMinor` → Page<AdminUserRow> |
| GET | `/admin/users/:id` | `users.read` | → AdminUserDetail (view is audited) |
| POST | `/admin/users/:id/suspend` | `users.write` | `{reason}` → AdminUserRow |
| POST | `/admin/users/:id/unsuspend` | `users.write` | `{}` → AdminUserRow |
| POST | `/admin/users/:id/guardian-consent/revoke` | `users.write` | `{reason}` → `{userId, consentId, guardianConsentStatus: "revoked"}` (audited; emits `guardian.consent_revoked`) |
| GET | `/admin/organisers` | `organisers.review` | `?status=submitted` → Page<`{user: UserSummary, orgName, orgType, status, submittedAt}`> |
| GET | `/admin/organisers/:userId` | `organisers.review` | → OrganiserVerification + `{user: UserSummary}` (document `url`s are signed) |
| POST | `/admin/organisers/:userId/approve` | `organisers.review` | `{note?}` → same (emits `organiser.approved`) |
| POST | `/admin/organisers/:userId/reject` | `organisers.review` | `{note}` → same (emits `organiser.rejected`) |

```jsonc
// AdminUserRow
{"id": 42, "displayName": "...", "mobileMasked": "******3210", "primaryRole": "player", "roles": ["player"],
 "status": "active", "verificationStatus": "verified", "isMinor": false,
 "guardianConsentStatus": "not_required", "onboardingStep": "done", "createdAt": "...", "lastLoginAt": "..."}
// AdminUserDetail = AdminUserRow + 
{"verification": {"status": "verified", "provider": "digilocker_direct", "aadhaarLast4": "1234", "verifiedName": "Asha Kulkarni" /* decrypted, view audited */, "gender": "F", "verifiedAt": "..."}|null,
 "guardian": {"relation": "mother", "mobileMasked": "******1111", "status": "granted", "grantedAt": "..."}|null,
 "counts": {"eventsOrganised": 0, "registrations": 0, "posts": 0, "reportsAgainst": 0},
 "suspendedReason": null}
```

`q` matches display name (prefix), user id (`#42`), or an exact mobile number (looked up by HMAC hash).

### Events, payments, plans, moderation, ads (modules `event`, `listingpayment`, `subscription`, `moderation`, `ads`)

| Method | Path | Perm | Body → Response |
|---|---|---|---|
| GET | `/admin/events` | `events.review` | `?status&sportId&q` → Page<EventSummary + {listing}> |
| GET | `/admin/events/:id` | `events.review` | → Event + `{listingOrders: [ListingOrder]}` |
| POST | `/admin/events/:id/approve` | `events.review` | `{}` → Event (`pending_review` → `published`) |
| POST | `/admin/events/:id/reject` | `events.review` | `{reason}` → Event (refund `listing.refund.on_admin_rejection_percent`) |
| POST | `/admin/events/:id/remove` | `events.review` | `{reason}` → Event (`published` → `removed`, emits `event.cancelled`) |
| POST | `/admin/events/:id/feature` | `events.review` | `{}` → Event (published only; repeat is a no-op) |
| POST | `/admin/events/:id/unfeature` | `events.review` | `{}` → Event |
| GET | `/admin/events/:id/registrations` | `events.review` | `?status&flagged=true&cursor` → Page<organiser registration view, with `qualifiedLabel` and `eligibilityFlag`> |
| GET | `/admin/listing-orders` | `payments.read` | `?status&eventId&from&to` → Page<ListingOrder + {organiser: UserSummary}> |
| POST | `/admin/listing-orders/:id/refund` | `payments.refund` | `{amountPaise, reason}` → ListingOrder |
| GET | `/admin/fee-rules` | `config.read` | → `[FeeRule]` |
| POST | `/admin/fee-rules` | `fees.write` | FeeRule (no id) → FeeRule |
| PUT | `/admin/fee-rules/:id` | `fees.write` | FeeRule (partial) → FeeRule (only future quotes change) |
| POST | `/admin/fee-rules/preview` | `config.read` | `{sportId, eventType, entryFeePaise, maxSlots}` → ListingQuote |
| GET | `/admin/plans` | `config.read` | → `[Plan + {active, sortOrder}]` |
| POST | `/admin/plans` | `plans.write` | plan → Plan |
| PUT | `/admin/plans/:id` | `plans.write` | plan (partial) → Plan |
| PUT | `/admin/plans/:id/benefits` | `plans.write` | `[{key, value, label, sortOrder}]` → Plan (replaces the list; keys from the closed list) |
| GET | `/admin/subscriptions` | `payments.read` | `?userId&status&planId` → Page<Subscription + {user: UserSummary}> |
| POST | `/admin/subscriptions/grant` | `plans.write` | `{userId, planId, days, reason}` → Subscription (store `manual`) |
| GET | `/admin/reports` | `reports.read` | `?status&targetType&reason` → Page<AdminReport> |
| POST | `/admin/reports/:id/resolve` | `reports.resolve` | `{action: "dismiss|hide|remove|suspend_user|warn", note}` → AdminReport |
| GET | `/admin/moderation/actions` | `reports.read` | `?targetType&targetId` → Page<ModerationAction> |
| POST | `/admin/moderation/actions` | `reports.resolve` | `{targetType, targetId, action: "hide|unhide|remove|suspend_user|unsuspend_user|warn", reason}` → ModerationAction |
| GET | `/admin/ads` | `ads.write` | → Page<AdCampaign> |
| POST | `/admin/ads` | `ads.write` | AdCampaign (no id) → AdCampaign |
| PUT | `/admin/ads/:id` | `ads.write` | partial → AdCampaign |
| GET | `/admin/ads/:id/stats` | `ads.write` | `?from&to` → `[{date, impressions, clicks}]` |

```jsonc
// FeeRule
{"id": 1, "name": "Default", "sportId": null, "eventType": null, "percentBp": 500,
 "minFeePaise": 9900, "maxFeePaise": 499900, "freeEventFeePaise": 9900, "gstBp": 1800,
 "priority": 0, "active": true, "effectiveFrom": "...", "effectiveTo": null}
// AdminReport
{"id": 1, "reporter": UserSummary, "targetType": "post", "targetId": 70,
 "targetPreview": {"title": "...|null", "text": "first 200 chars|null", "ownerId": 42},
 "reason": "spam", "details": "...", "status": "open|actioned|dismissed",
 "openReportsOnTarget": 3, "resolutionNote": null, "createdAt": "...", "resolvedAt": null}
// ModerationAction
{"id": 1, "admin": {"id": 1, "name": "..."}|null /* null = automatic */, "targetType": "post", "targetId": 70,
 "action": "hide", "reason": "...", "reportId": 1, "createdAt": "..."}
// AdCampaign
{"id": 1, "advertiserName": "...", "title": "...", "body": null, "imageUploadId": null, "imageUrl": null,
 "targetUrl": "https://...", "placement": "home_feed", "sportId": null, "state": null, "priority": 0,
 "startsAt": "...", "endsAt": "...", "status": "draft|active|paused|ended"}
```

Admin image uploads (ad images) use `POST /admin/media/uploads` with the same body as
`/media/uploads` (perm `ads.write`, purpose `post_image`), owned by user id 0.

### Communities, messages, broadcast (modules `community`, `chat`, `notification`)

| Method | Path | Perm | Body → Response |
|---|---|---|---|
| GET | `/admin/communities` | `communities.moderate` | `?q&status` → Page<Community + {status}> |
| POST | `/admin/communities/:id/remove` | `communities.moderate` | `{reason}` → 204 |
| GET | `/admin/conversations/:id/messages` | `moderation.read_messages` | `?before` → Page<Message>. **Only for a conversation with an open report on it or one of its messages**, else 404. Audited. |
| POST | `/admin/notifications/broadcast` | `notifications.broadcast` | `{title, body, audience: {roles?: [], sportId?, state?, includeMinors: false}}` → `{queued: 1234}` |

### Location identities and districts (module `locationidentity`)

| Method | Path | Perm | Body → Response |
|---|---|---|---|
| GET | `/admin/location-identities` | `location.review` | `?status=pending&identityType&stateId&districtId` → Page<AdminLocationRow> (pending: oldest first) |
| GET | `/admin/location-identities/:id` | `location.review` | → AdminLocationDetail (proof view is audited) |
| POST | `/admin/location-identities/:id/approve` | `location.review` | `{aadhaarLast4?: "1234", note?}` → AdminLocationDetail |
| POST | `/admin/location-identities/:id/reject` | `location.review` | `{rejectionCode, rejectionReason?}` → AdminLocationDetail |
| POST | `/admin/location-identities/:id/revoke` | `location.review` | `{reason}` → AdminLocationDetail (verified → revoked, e.g. fraud) |
| GET | `/admin/geo/states` | `config.read` | → `[State + {lgdCode, active}]` (inactive states too, by name) |
| GET | `/admin/geo/districts` | `config.read` | `?stateId&q&includeInactive` → Page<AdminDistrict> |
| POST | `/admin/geo/districts` | `geo.write` | `{stateId, name, lgdCode?, aliases?: []}` → AdminDistrict |
| PUT | `/admin/geo/districts/:id` | `geo.write` | `{name?, lgdCode?, aliases?, active?}` → AdminDistrict |

```jsonc
// AdminLocationRow
{"id": 5, "user": UserSummary, "isMinor": false, "identityType": "residence", "district": District,
 "state": State, "proofType": "rent_agreement", "proofHolder": "self", "status": "pending", "submittedAt": "..."}
// AdminLocationDetail = AdminLocationRow + 
{"proofUrl": "https://...signed, location.proof_view_url_ttl_seconds...|null" /* null after purge */,
 "proofContentType": "application/pdf|image/jpeg|image/png|null", "proofPurgedAt": "...|null",
 "aadhaarLast4": null, "rejectionCode": null, "rejectionReason": null,
 "reviewedAt": null, "reviewedBy": {"id": 2, "name": "Ops"}|null, "verifiedAt": null, "expiresAt": null,
 "profileDistrict": "Thanjavur|null" /* profile text, a hint only */,
 "history": [LocationIdentity] /* the user's earlier rows of this type */,
 "checklist": {"utilityBillMaxAgeDays": 90, "rejectionReasons": ["unclear_document", "..."]}}
// AdminDistrict = District + {"lgdCode": 616|null, "aliases": ["Tanjore"], "active": true}
```

- Approve: only `pending`. The user's verified row of the same type (if any) → `superseded`;
  residence gets `expiresAt = now + location.residence_validity_months`. `aadhaarLast4` only for
  `masked_aadhaar` (exactly 4 digits, else 422). Reviewer must not be the user (admins are separate accounts).
- Reject: `rejectionCode` in `location.rejection_reasons` (422); `unmasked_aadhaar` deletes the
  proof file immediately. Revoke: only `verified`.
- All three write `admin_audit_log` in the same transaction and emit `location_identity.changed`.
  Opening the detail (signed proof URL) writes an audit row `location.proof_viewed`.
- District edits: `config_audit` row; the eligibility places cache refreshes within 10 minutes.
  Deactivating a district hides it from pickers; existing identities and events keep it.
  Bulk load: `make import-districts FILE=lgd.csv` (`cmd/admincli import-districts`; LGD CSV
  columns `state_code,district_name,lgd_code[,aliases]`, upsert by `lgd_code`, then by name).
