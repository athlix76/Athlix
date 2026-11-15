# features/admins

Admin users and roles.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `AdminsScreen` | `/admins` | Admins tab (create, edit role / status, reset credentials) and Roles tab (create, edit permissions) |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/admins` | admins.manage |
| POST | `/admin/admins` | admins.manage |
| PUT | `/admin/admins/:id` | admins.manage |
| POST | `/admin/admins/:id/reset-credentials` | admins.manage |
| GET | `/admin/roles` | admins.manage |
| POST | `/admin/roles` | admins.manage |
| PUT | `/admin/roles/:id` | admins.manage |

## Permissions

`admins.manage`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Temporary passwords and TOTP URIs are shown once in `one_time_credentials_dialog.dart` and dropped when it closes. Never store, log or copy them into state that outlives the dialog.
- The permission checkboxes come from `Perm.values`; add new permissions there (and in docs/security.md via its owner).
- You cannot disable yourself; the dialog hides that option and the backend refuses it.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.
