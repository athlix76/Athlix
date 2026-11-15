# lib/core/auth

Admin identity: permissions (`Perm`, `hasPermission`), the `AdminMe` model, the in-memory token store and the idle watcher.

| File | What it is |
|---|---|
| `admin_me.dart` | `AdminMe` / `AdminUser` from docs/api.md. |
| `idle_watcher.dart` | Calls [onIdle] after [timeout] without pointer or keyboard input. |
| `permissions.dart` | Admin permissions from docs/security.md (Admin permissions). |
| `token_store.dart` | `TokenPair` from docs/api.md. |

## Changing it safely

- Tokens stay in memory only (security.md): no localStorage, sessionStorage, cookies or IndexedDB. A reload signs the admin out, by design.
- New permissions are added to `Perm` and `Perm.values` together, matching docs/security.md exactly.
- Covered by test/core/permissions_test.dart.
