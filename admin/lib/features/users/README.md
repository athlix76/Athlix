# features/users

Find users, view one, suspend and unsuspend, revoke a minor's guardian consent.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `UsersScreen` | `/users` | search and filters |
| `UserDetailScreen` | `/users/:id` | profile, verification, guardian (revoke consent for minors with granted consent), suspend |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/users` | users.read |
| GET | `/admin/users/:id` | users.read |
| POST | `/admin/users/:id/suspend` | users.write |
| POST | `/admin/users/:id/unsuspend` | users.write |
| POST | `/admin/users/:id/guardian-consent/revoke` | users.write |

## Permissions

View `users.read`; suspend / unsuspend and revoke-consent buttons need `users.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- The verified name from the detail GET is hidden until the admin asks; the reveal re-fetches the detail (audited by the backend). Do not keep it in a provider.
- Show only masked PII (`mobileMasked`). Never log the detail response.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.
