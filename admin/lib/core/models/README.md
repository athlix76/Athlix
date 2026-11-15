# lib/core/models

Small models shared by several features (`UserSummary`, `AdminRef`, `SportRef`, listing quote).

| File | What it is |
|---|---|
| `common.dart` | `SportRef` from docs/api.md. |
| `listing.dart` | `ListingQuote` from docs/api.md (listing fee module). |

## Changing it safely

- A model used by one feature belongs in that feature's `data/` folder, not here.
