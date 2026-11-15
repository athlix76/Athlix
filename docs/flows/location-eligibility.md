# Location eligibility

Some tournaments are only for players **from** a place (native, proven by official ID) or
**living in** it (residence, proven by an address proof), at district or state level. The
organiser sets `category` (`government` or `private`) and an `eligibility` rule on the event;
the server checks the player's **location identities** against it in the feed, on the event
page, before revealing payment details, and inside the registration transaction.

- One rule implementation: `platform/eligibility`. Event, registration and search all call it.
- Only the **district** (and its state) is kept, never the address. Aadhaar: last 4 digits at most.
- Proof files are visible only to admins with `location.review`, through short signed URLs
  (every view is audited), and are deleted `location.proof_retention_days` after review.
- Registrations are **flagged, never cancelled**, when the qualifying identity stops being valid.
- `location.enabled = false` turns the feature off: every event behaves as `open`.

Contract: [api.md](../api.md) sections "Location identity" and "Location identities and
districts", [architecture.md](../architecture.md) "Location eligibility",
[config-system.md](../config-system.md) `location.*`.

## 1. Native identity from DigiLocker

When DigiLocker verification succeeds, the Aadhaar address district is matched to the
`districts` table (by name or alias) and a verified native identity is created automatically.

```mermaid
sequenceDiagram
    autonumber
    actor U as Player
    participant App as Athlix app
    participant API as Athlix API
    participant DL as DigiLocker
    participant W as Worker

    U->>App: Verify with DigiLocker
    App->>API: POST /verification/digilocker/start {}
    API-->>App: {authorizationUrl, expiresAt}
    U->>DL: Log in and consent
    DL->>API: GET /verification/digilocker/callback?code&state
    API->>DL: Exchange code and fetch Aadhaar-derived fields
    Note over API: Match the address district by name or alias. Keep only the district id. Drop the address
    API-->>App: 302 athlix://verification/result?status=verified
    Note over API: emits verification.completed with nativeDistrictId
    W->>W: locationidentity creates a verified native identity, source digilocker, no document, no expiry
    Note over W: emits location_identity.changed
    alt District could not be matched
        Note over W: nativeDistrictId is null, no identity is created
        App->>API: GET /me/location-identities
        API-->>App: native displayStatus not_submitted
        Note over App: The player uploads a native proof instead, see section 2
    end
```

- Native identities never expire. A later reviewed native change (for example a birth
  certificate) is allowed only when `location.native_change_allowed` is true (else 409
  `INVALID_STATE`); on approval the old row becomes `superseded`.

## 2. Upload, review, verified, expiry and renewal

The same flow is used for a native upload (`location.native_proof_types`) and for residence
(`location.residence_proof_types`). Residence expires; native does not.

```mermaid
sequenceDiagram
    autonumber
    actor U as Player
    participant App as Athlix app
    participant API as Athlix API
    participant S3 as S3 private media bucket
    actor A as Reviewer admin
    participant W as Worker

    App->>API: GET /me/location-identities
    API-->>App: LocationIdentities with slots, proofTypes, notice, guardianProofAllowed
    App->>API: GET /geo/states then GET /geo/districts?stateId
    U->>App: Read the notice, choose district and proof type
    App->>API: POST /media/uploads {purpose location_proof, contentType, sizeBytes}
    API-->>App: UploadTicket with uploadUrl, the backend builds the key
    App->>S3: PUT file bytes
    App->>API: POST /media/uploads/:id/confirm {}
    App->>API: POST /me/location-identities LocationIdentityInput
    alt Second pending of the same type
        API-->>App: 409 CONFLICT
    else Residence change to another district inside the cooldown
        API-->>App: 409 LIMIT_REACHED with fields.nextChangeAt
    else Wrong notice, upload, proof type, inactive district or guardian not allowed
        API-->>App: 422 VALIDATION_FAILED
    else Accepted
        API-->>App: LocationIdentity with status pending
        Note over API: notice acceptance recorded, emits location_identity.changed
    end

    A->>API: GET /admin/location-identities?status=pending
    API-->>A: Page of AdminLocationRow, oldest first
    A->>API: GET /admin/location-identities/:id
    API-->>A: AdminLocationDetail with a short signed proofUrl and the checklist
    Note over API: audit row location.proof_viewed
    alt Approve
        A->>API: POST /admin/location-identities/:id/approve {aadhaarLast4 only for masked_aadhaar, note}
        API-->>A: verified, earlier verified row of the same type superseded
        Note over API: residence gets expiresAt from location.residence_validity_months
    else Reject
        A->>API: POST /admin/location-identities/:id/reject {rejectionCode, rejectionReason}
        API-->>A: rejected
        Note over API: unmasked_aadhaar deletes the proof file at once
    end
    Note over API: admin_audit_log in the same transaction, emits location_identity.changed
    W->>U: Notification verified or rejected

    W->>W: Job location.expiry_reminders daily 10:00 IST
    W->>U: Notification renew residence, from location_identity.expiring
    U->>App: Renew for the same district, allowed any time
    Note over App: Same submit flow as above
    W->>W: Job location.expire_residence daily 00:30 IST
    Note over W: verified residence past expiresAt becomes expired, emits location_identity.changed
```

```mermaid
stateDiagram-v2
    [*] --> pending: submit upload
    [*] --> verified: DigiLocker native
    pending --> verified: admin approve
    pending --> rejected: admin reject
    pending --> withdrawn: user withdraws
    verified --> superseded: newer row of the same type approved
    verified --> expired: residence past expiresAt
    verified --> revoked: admin revoke
    verified --> withdrawn: user withdraws
    rejected --> [*]
    expired --> [*]
    superseded --> [*]
    revoked --> [*]
    withdrawn --> [*]
```

- A user has at most one verified and one pending identity per type (unique slots in the DB).
- Minors may submit a parent or guardian address proof for residence
  (`proofHolder: guardian`) when `location.minor_guardian_proof_allowed` is true.
- Revoke (`POST /admin/location-identities/:id/revoke {reason}`) is only for verified rows,
  for example fraud. Withdraw is `DELETE /me/location-identities/:id`; it deletes the proof at once.
- Reviewer checklist values come from config: `location.utility_bill_max_age_days`,
  `location.rejection_reasons`.

## 3. Feed filter and showAll

```mermaid
sequenceDiagram
    autonumber
    actor U as Player
    participant App as Athlix app
    participant API as Athlix API
    participant E as platform/eligibility

    App->>API: GET /events?sportId&lat&lng&radiusKm&sort=distance
    API->>E: Feed filter SQL for the caller's identities
    E-->>API: Condition with bind params, or always true when location.enabled is false
    API-->>App: Page of EventSummary, open events and events the player is eligible for
    Note over App: Each card shows myEligibility.tag such as Eligible via Native Thanjavur
    U->>App: Show all events
    App->>API: GET /events?showAll=true
    API-->>App: Also the other events with myEligibility.eligible false and an action
    Note over App: Greyed card with Verify now, the action is verify_native, verify_residence, renew_residence or wait_for_review
    App->>API: GET /events/:id
    API-->>App: Event with myEligibility, paymentInstructions null when not eligible
```

- `lat` and `lng` are sent per request and never stored. `radiusKm` is 1 to
  `event.max_search_radius_km`.
- The same filter applies to `GET /events/featured` and search.

## 4. Register check and NOT_ELIGIBLE

```mermaid
sequenceDiagram
    autonumber
    actor U as Player
    participant App as Athlix app
    participant API as Athlix API
    participant E as platform/eligibility

    App->>API: GET /events/:id/contact
    API->>E: Check the caller against the event rule
    alt Not eligible
        API-->>App: 403 NOT_ELIGIBLE with fields.reason and fields.action
    else Eligible
        API-->>App: EventContact
    end
    U->>App: Register
    App->>API: POST /events/:id/registrations {teamName, teamMembers}
    Note over API: Transaction starts, identities read in the same transaction
    API->>E: Check the caller against the event rule
    alt No identity, still pending under requireVerified, expired or wrong place
        API-->>App: 403 NOT_ELIGIBLE, reason identity_missing, identity_pending, identity_expired or wrong_location
        Note over App: Show the reason and a button for fields.action
    else Eligible
        API-->>App: Registration with qualifiedVia and qualifiedDistrict
        Note over API: emits registration.created
    end
```

- `requireVerified: false` lets a pending identity qualify; if that pending row is later
  rejected, section 5 applies.
- Waitlist promotion re-checks eligibility; a waitlisted player who is no longer eligible is
  skipped and flagged.
- Only the registrant is checked. Team members are names only.

## 5. Identity change flags registrations, never cancels

```mermaid
sequenceDiagram
    autonumber
    participant API as Athlix API
    participant W as Worker
    participant E as platform/eligibility
    actor O as Organiser
    actor P as Player

    Note over API: Identity expired, revoked, withdrawn, superseded, or a pending one used under requireVerified false is rejected
    API->>W: location_identity.changed
    W->>W: registration loads registered and waitlisted rows of events not yet started that used this identity
    loop Each affected registration
        W->>E: Evaluate with the player's remaining identities
        alt Still eligible through another identity
            W->>W: Update qualifiedVia and qualifiedDistrict silently
        else Not eligible any more
            W->>W: Set eligibilityFlag and eligibilityFlaggedAt
            Note over W: emits registration.eligibility_flagged
            W->>O: Notification registration flagged
            W->>P: Notification renew or verify
        end
    end
    O->>API: GET /events/:id/registrations?flagged=true
    API-->>O: RegistrationForOrganiser with qualifiedLabel and eligibilityFlag
    alt Organiser decides to remove the player
        O->>API: PATCH /events/:id/registrations/:regId {status rejected}
    else Organiser keeps the player
        Note over O: Nothing changes, the registration stays
    end
```

- `eligibilityFlag` values: `identity_expired`, `identity_rejected`, `identity_revoked`,
  `identity_changed`.
- Off when `location.flag_registrations_on_change` is false. The system never cancels a
  registration on its own.

## 6. Proof purge

```mermaid
sequenceDiagram
    autonumber
    participant W as Worker
    participant DB as MySQL
    participant S3 as S3 private media bucket

    W->>W: Job location.purge_proofs daily 02:30 IST
    W->>DB: Reviewed rows older than location.proof_retention_days and withdrawn rows that still have a proof
    loop Each row
        W->>S3: media.Purge deletes the object, idempotent
        W->>DB: Set proof_purged_at, proof_upload_id NULL
    end
    Note over DB: The status, district, proof type and dates stay. The document is gone
```

- Seed default: 30 days (`location.proof_retention_days`).
- Deleted earlier: on rejection with `unmasked_aadhaar`, on withdrawal, and when the account is
  deleted (`user.deleted`).
- After the purge the admin detail returns `proofUrl: null` and `proofPurgedAt`.

Author: Atindraa.
