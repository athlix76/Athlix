# features/onboarding

Role choice and profile setup after sign-in.

## Endpoints

- `POST /onboarding/role`
- `POST /onboarding/profile`
- `POST /me/roles (settings, add a role)`

## Rules

- The next step comes from the server (`onboardingStep` / `fields.nextStep`).

## Changing it safely

Do not add client-side step skipping; the server decides.
