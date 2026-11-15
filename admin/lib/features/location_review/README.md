# features/location_review

Location identity review queue (tournament eligibility): proof viewer, approve, reject, revoke.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `LocationReviewScreen` | `/location-review` | queue, default status pending, oldest first |
| `LocationDetailScreen` | `/location-review/:id` | summary, reviewer checklist, proof, history, actions |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/location-identities` | location.review |
| GET | `/admin/location-identities/:id` | location.review (audited as location.proof_viewed) |
| POST | `/admin/location-identities/:id/approve` | location.review |
| POST | `/admin/location-identities/:id/reject` | location.review |
| POST | `/admin/location-identities/:id/revoke` | location.review |
| GET | `/admin/geo/districts` | config.read (district filter only) |

## Permissions

`location.review`. The district dropdown in the filter also needs `config.read`; without it the filter takes a district id.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- `proof_viewer.dart` hides the proof when the signed URL expires (`application/proof_link.dart`) and evicts it from the image cache. "Show again" re-fetches the detail, which is a new audited view.
- Never download, copy or log the proof URL. The page sets `no-referrer` so it cannot leak through the Referer header.
- The approve note is internal (at most 255, stored as `review_note`) and shown as "Internal note" in the detail; it is never shown to the user. The reject note is shown to the user.
- Aadhaar: only the last 4 digits are entered, and only for `masked_aadhaar`. Checklist numbers come from the detail (`utilityBillMaxAgeDays`, `rejectionReasons`), never from code.
- See docs/flows/location-eligibility.md.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.
