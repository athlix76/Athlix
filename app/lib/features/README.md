# lib/features

One folder per product area. Each has `data/` (API + models), `application/` (providers, controllers, pure
view logic) and `presentation/` (screens, widgets), plus a README listing its screens and endpoints.

| Feature | What |
|---|---|
| `startup` | splash, force update, maintenance, suspended |
| `auth` | phone + OTP sign-in |
| `policies` | policy reading and acceptance |
| `onboarding` | role choice and profile setup |
| `verification` | DigiLocker identity verification (plus mock in non-prod) |
| `guardian` | guardian consent for minors |
| `location_identity` | native / residence identity, eligibility view logic, NOT_ELIGIBLE sheet |
| `home` | tab shell and home screen |
| `events` | browse, discovery bar, detail, create / edit (with eligibility), my events |
| `registrations` | register, my registrations, organiser registrations (flags) |
| `listing_payment` | listing-fee quote, order and checkout |
| `feed`, `follow`, `profile`, `search` | social |
| `chat`, `communities` | messaging and groups |
| `notifications` | in-app list and push |
| `premium`, `consultations`, `organiser`, `settings`, `ads`, `reports` | the rest |

## Changing it safely

Follow the layout and README rule for any new feature (see `app/README.md`, "Adding a feature").
