# Minor sign-up and guardian consent

Minors may use Athlix with a parent's or guardian's consent. A user is a minor when the
DigiLocker DOB gives an age below `guardian.minor_age_years` (default 18, the DPDP definition
of a child; do not lower it). Below `auth.min_age_years` (default 10) the user is refused.
The requirement comes from `verification_requirements` (`guardian_consent_if_minor` /
`app_access`, required for every role by default).

The consent OTP goes to the **guardian's** mobile, which must differ from the minor's own
mobile. The guardian's name and mobile are stored encrypted (`guardian_consents`, `_enc`,
`_hash`, `_last4`), with the relation, the consent policy version and the time.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    actor M as Minor
    actor G as Guardian
    participant App as Athlix app
    participant API as Athlix API
    participant SMS as SMS provider

    Note over App,API: After DigiLocker returned status=minor, Me.onboardingStep is guardian
    App->>API: GET /policies/guardian_consent
    API-->>App: PolicyDocument {id, version, contentMd}
    M->>App: Show consent text to the guardian, enter guardian name, mobile, relation
    App->>API: POST /onboarding/guardian/request {guardianName, guardianMobile, relation, policyDocumentId}
    alt Guardian mobile equals the minor's mobile
        API-->>App: 422 VALIDATION_FAILED
    else Valid
        API->>SMS: Send guardian_otp template to the guardian mobile
        API-->>App: {consentId, guardianMobileMasked, expiresInSeconds, resendAfterSeconds}
    end
    SMS-->>G: OTP with the consent message
    G->>App: Reads the consent text and enters the OTP on the minor's phone
    App->>API: POST /onboarding/guardian/verify {consentId, otp}
    alt Wrong or expired OTP
        API-->>App: 422 VALIDATION_FAILED
    else Correct
        Note over API: consent granted, guardianConsentStatus granted, step profile
        API-->>App: Me
        Note over API: emits guardian.consent_granted after commit
    end
    App->>API: POST /onboarding/profile ProfileSetup
    API-->>App: Me with onboardingStep done
```

The OTP is valid for `guardian.consent_otp_ttl_seconds` (default 600).

## Consent status

```mermaid
stateDiagram-v2
    [*] --> not_required: adult
    [*] --> pending: verified as minor
    pending --> granted: guardian OTP verified
    granted --> revoked: consent withdrawn
    revoked --> pending: new consent requested
    granted --> not_required: turns 18, job verification.minor_recheck
```

## Withdrawing consent

```mermaid
sequenceDiagram
    participant M as Minor's app
    participant API
    participant G as Guardian's phone
    M->>API: POST /me/guardian/withdraw/request
    API->>G: SMS code (guardian_otp template)
    API-->>M: {consentId, guardianMobileMasked, expiresInSeconds}
    M->>API: POST /me/guardian/withdraw/verify {consentId, otp}
    API-->>M: Me (guardianConsentStatus revoked, onboardingStep guardian)
    API--)API: guardian.consent_revoked → notification
```

- Only a minor whose consent is `granted` can start it (Settings → "Withdraw guardian
  consent"). The code goes to the **guardian's** mobile, so the minor cannot withdraw alone.
- On success the consent row gets `revoked_at` and `revoked_reason = guardian_withdrew`, and the
  app's router sends the user back to the guardian step. Until a new consent is granted the
  account cannot use verified features.
- Support can revoke consent too: `POST /admin/users/:id/guardian-consent/revoke {reason}`
  (`users.write`, audited), shown on the admin user page for minors with granted consent.

## What changes for a minor (enforced in the backend)

| Rule | Config | Default |
|---|---|---|
| Who can DM a minor (checked in both directions) | `minor.dm_policy` | `mutual_follow` |
| Consultation DM exception | (none) | never applies to minors |
| Default profile visibility, `public` refused | `minor.profile_default_visibility` | `followers` |
| City hidden from non-followers | (fixed rule) | on |
| Appears in people search, without photo, city and bio unless mutual follow | `minor.searchable` | `true` |
| May join public communities; invites follow the DM rule | `minor.can_join_public_communities` | `true` |
| Sponsored placements | `ads.show_to_minors` | `false` (keep it) |
| Profile views by a minor are not recorded | (fixed rule) | on |
| Posts default to followers when the profile default is not public | (fixed rule) | on |
| `underage_safety` reports first in the admin queue | (fixed rule) | on |

## Open points

- An OTP proves control of the guardian's mobile, not that the guardian is an adult. Whether
  this is "verifiable" parental consent under the DPDP Rules is for the lawyer
  ([compliance.md](../business/compliance.md#childrens-data)).
