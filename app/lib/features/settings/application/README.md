# settings/application

- `settings_controller.dart`:
  - `SettingsController` (optimistic updates, rolled back on error)
  - `AccountActions` (add role, log out, delete account)
  - `AddableRole`

Keep the push stop before the session calls in `logout` / `deleteAccount`.
