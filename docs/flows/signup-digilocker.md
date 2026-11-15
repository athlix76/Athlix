# Sign-up and DigiLocker verification

A new user signs in with a mobile OTP, picks a role, verifies identity and age with DigiLocker,
and completes a profile. The **server** does the DigiLocker OAuth2 code exchange (with PKCE);
the app only opens a browser tab and receives a deep link. Athlix keeps the Aadhaar **last 4
digits**, encrypted name and DOB, gender, and a keyed hash of the provider subject id. The full
Aadhaar number, XML, PDF, photo and the DigiLocker access token are thrown away.

Whether a role needs DigiLocker comes from `verification_requirements` (`digilocker` /
`app_access`, required for every role by default).

## Onboarding steps

```mermaid
stateDiagram-v2
    [*] --> role: first OTP verify, isNewUser true
    role --> verification: POST /onboarding/role
    verification --> guardian: verified and isMinor
    verification --> profile: verified adult
    verification --> verification: failed, retry within daily limit
    guardian --> profile: guardian consent granted
    profile --> done: POST /onboarding/profile
    done --> [*]
```

If DigiLocker is not required for the user's roles, `role` goes straight to `profile`. Until
the step is `done`, every `verified` route answers 403 `ONBOARDING_REQUIRED` with
`fields.nextStep`.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    actor U as User
    participant App as Athlix app
    participant API as Athlix API
    participant R as Redis
    participant SMS as SMS provider
    participant DL as DigiLocker

    U->>App: Enter mobile number
    App->>API: POST /auth/otp/request {mobile}
    API->>R: Rate limit per mobile hash and IP, store OTP hash with TTL
    API->>SMS: Send OTP
    API-->>App: 200 {requestId, resendAfterSeconds, expiresInSeconds}
    U->>App: Enter OTP
    App->>API: POST /auth/otp/verify {requestId, otp, deviceId}
    API-->>App: {tokens, user, isNewUser true} and emits user.registered
    App->>API: POST /onboarding/role {role}
    API-->>App: Me with onboardingStep verification

    App->>API: POST /verification/digilocker/start {}
    API->>R: Store one-time state bound to the user, PKCE verifier
    API-->>App: {authorizationUrl, expiresAt}
    App->>DL: Open authorizationUrl in a browser tab
    U->>DL: Log in and consent to share
    DL->>API: GET /verification/digilocker/callback?code&state
    API->>R: Redeem state once, load user and PKCE verifier
    API->>DL: Exchange code for token, server to server with PKCE
    API->>DL: Fetch Aadhaar-derived identity fields
    Note over API: Keep last 4, encrypt name and DOB, keep gender, HMAC the subject id. Drop number, XML, PDF, photo, token
    alt Subject hash already linked to another account
        API-->>App: 302 athlix://verification/result?status=failed and reason=already_linked
    else Age below auth.min_age_years
        API-->>App: 302 athlix://verification/result?status=failed and reason=too_young
    else Age below guardian.minor_age_years
        API-->>App: 302 athlix://verification/result?status=minor
        Note over API: isMinor true, step guardian, emits verification.completed
    else Adult
        API-->>App: 302 athlix://verification/result?status=verified
        Note over API: step profile, emits verification.completed
    end
    App->>API: GET /verification/status
    API-->>App: VerificationStatus {status, aadhaarLast4, isMinor, attemptsLeftToday}
    App->>API: POST /onboarding/profile ProfileSetup
    API-->>App: Me with onboardingStep done
```

## Rules

- OTP: `auth.otp.length`, `auth.otp.ttl_seconds`, `auth.otp.max_attempts` wrong codes burn the
  request, `auth.otp.resend_cooldown_seconds`, `auth.otp.max_per_hour` per mobile and per IP.
  `otp/request` returns 200 for any valid number (does not reveal whether the user exists).
- The DigiLocker `state` is single-use, bound to the user and valid
  `verification.state_ttl_seconds`. Attempts are limited by
  `verification.max_attempts_per_day` (429).
- The provider is chosen by `verification.provider`: `digilocker_direct`, `aggregator` or
  `mock`. `mock` (`POST /verification/mock/complete`) works only when ENV is not prod.
- `profiles.birth_year` is set from the verified DOB and used for age checks; the job
  `verification.minor_recheck` clears `is_minor` after the 18th birthday.
- Minors continue at [minor-consent.md](minor-consent.md).
- New policy versions that need acceptance appear in `Me.pendingPolicies`; the app blocks
  until `POST /me/policies/accept`.
