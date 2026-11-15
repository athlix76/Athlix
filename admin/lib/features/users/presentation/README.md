# features/users/presentation

Screens, dialogs and widgets for this feature.

| File | What it is |
|---|---|
| `user_detail_screen.dart` | One user: profile, verification, guardian, audited verified-name reveal, suspend / unsuspend. |
| `users_screen.dart` | User search. |

## Changing it safely

Every write goes through `confirmAction` or `showEditorDialog` so the admin confirms and sees API error codes. Gate buttons with `sessionProvider.can(...)`.
See `../README.md` for the endpoints and permissions.
