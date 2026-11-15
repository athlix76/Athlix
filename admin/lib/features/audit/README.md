# features/audit

Read-only audit log and admin sign-in attempts.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `AuditLogScreen` | `/audit-log` | filters by admin, action, target; details as JSON |
| `LoginEventsScreen` | `/login-events` | admin sign-ins: filter by admin id, email tried, outcome; failure reason, IP, user agent |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/audit-log` | audit.read |
| GET | `/admin/login-events` | audit.read |

## Permissions

`audit.read`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Details are shown as the backend sends them; the backend keeps PII out of them.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.
