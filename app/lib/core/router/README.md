# core/router

- `routes.dart`: every path and path builder (e.g. `Routes.event(id)`, `Routes.locationIdentitySubmit(type)`).
- `app_router.dart`: go_router setup, bottom-tab shell, all screen registrations.
- `redirect.dart`: `computeRedirect`, the single routing rule (gates, sign-in, policies, onboarding, deep link).

## Changing it safely

New screen: add the constant, register it in `app_router.dart`, and decide whether onboarding users may see it
(update `computeRedirect` and `test/redirect_test.dart` if so). Never put PII in paths or query params; use ids.
