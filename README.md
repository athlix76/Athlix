# Athlix

A sports community for India: players, coaches and organisers build verified profiles,
discover and list tournaments and trials, chat 1:1 and in community groups, and follow each
other's progress.

- **Verified by DigiLocker.** Every user verifies age and identity; Athlix keeps only the last 4
  Aadhaar digits and an encrypted name/DOB. Minors join with parent OTP consent.
- **Athlix never holds entry fees.** Players pay organisers directly (cash, UPI, bank, link);
  the organiser's contact is on the event. Athlix earns a **listing fee** (a configurable % of
  entry fee × slots, with min/max and GST), paid by the organiser through Razorpay, plus
  premium plans (in-app purchase) and contextual ads.
- **Everything business-related is config**, edited in the admin panel without a deploy:
  fees, refund rules, premium benefits, verification gates, organiser checklist, limits.

## Repository

| Folder | What | Start here |
|---|---|---|
| `backend/` | Go API, WebSocket, worker, migrations | [backend/README.md](backend/README.md) |
| `app/` | Flutter mobile app (Android, iOS) | [app/README.md](app/README.md) |
| `admin/` | Flutter Web admin panel | [admin/README.md](admin/README.md) |
| `infra/` | Terraform for AWS (ap-south-1) | [infra/README.md](infra/README.md) |
| `docs/` | API contract, architecture, config, security, AWS guide, business docs | [docs/README.md](docs/README.md) |

AI coding agents: read [CLAUDE.md](CLAUDE.md) (rules) first.

## Quick start (backend)

```bash
docker compose up -d                 # MySQL 8 + Redis 7 for local dev
cd backend && make dev-secrets migrate run
curl localhost:8080/v1/health
```

App (Android emulator; first generate `android/` and `ios/`, see [app/README.md](app/README.md)):
`cd app && flutter pub get && flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080/v1`.
Admin: `cd admin && flutter run -d chrome --web-port 5050 --release` (release mode: debug mode is
blocked by the panel's CSP; see [admin/README.md](admin/README.md)).
AWS: [docs/aws-setup.md](docs/aws-setup.md).

## Status

Company not registered yet. Before launch: [docs/business/launch-checklist.md](docs/business/launch-checklist.md)
(registration, DigiLocker partner onboarding, Razorpay KYC, lawyer-reviewed policies, DPDP).

Author: Atindraa.
