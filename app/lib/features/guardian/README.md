# features/guardian

Guardian consent for minors (guardian mobile + OTP).

## Endpoints

- `POST /onboarding/guardian/request`
- `POST /onboarding/guardian/verify`
- `POST /me/guardian/withdraw/request`, `POST /me/guardian/withdraw/verify` (used by Settings)

## Rules

- Minors' safety rules are enforced by the server.
- Never log the guardian mobile or OTP.

## Changing it safely

Age thresholds come from config (`guardian.minor_age_years`).
