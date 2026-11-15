# features/broadcast

One notification to an audience (roles, sport, state).

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `BroadcastScreen` | `/broadcast` | compose, audience, review and send |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| POST | `/admin/notifications/broadcast` | notifications.broadcast |

## Permissions

`notifications.broadcast`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- `includeMinors` is off by default and must stay so.
- A broadcast cannot be recalled; the confirm dialog shows the audience.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.
