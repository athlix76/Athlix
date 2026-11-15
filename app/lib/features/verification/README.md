# features/verification

DigiLocker identity verification. In non-prod builds a mock panel can complete it (outcome, and an optional native district for location eligibility testing).

## Endpoints

- `POST /verification/digilocker/start`
- `GET /verification/status`
- `POST /verification/mock/complete`

## Rules

- The app never sees Aadhaar data; only status.
- Mock UI only when `Env.showMockFlows`.

## Changing it safely

The deep link route is `/verification/result`; see `app/README.md`.
