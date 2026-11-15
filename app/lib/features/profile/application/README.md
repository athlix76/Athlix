# profile/application

- `profile_providers.dart`: `profileProvider` (family by user id, reloads on follow / block
  changes), `myProfileProvider`, `statsProvider`, `achievementsProvider`, and the template
  helpers `findTemplate` and `templatesForProfile`.
- `profile_controller.dart`: `ProfileController` for every write (profile, stats,
  achievements, uploads). It invalidates the matching providers and refreshes `Me` after a
  profile save.

Changing it safely: screens should write through the controller so lists refresh.
