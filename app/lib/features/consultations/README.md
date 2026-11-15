# features/consultations

Coach consultation offers and requests. Athlix **never collects consultation fees**:
`feeNote` is free text and players pay the coach directly.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `ConsultationsScreen` | `/consultations` | Received (coaches) and Sent tabs, status filter; coach accepts / declines / completes, player cancels pending |
| `ConsultationOfferScreen` | `/consultations/offer` | coach edits their own offer (`PUT /me/consultation`) |
| `CoachConsultationScreen` | `/coaches/:id/consultation` | a coach's offer and the request form |

## Endpoints

- `GET /coaches/:userId/consultation`, `PUT /me/consultation`
- `POST /coaches/:userId/consultation-requests`, `GET /me/consultation-requests?as&status&cursor`
- `POST /consultation-requests/:id/respond` `{accept, note?}`, `.../cancel`, `.../complete`

## Rules

- The open-request limit (`consultation.max_open_requests_per_player`) is shown from config;
  the server returns 409 past it.
- Listing an offer may need a premium benefit: 403 `PLAN_REQUIRED` shows `PlanRequiredView`.

## Changing it safely

New status or action: add it to `ConsultationStatus` and to `_RequestCard`, and patch the row
with the returned request rather than reloading the list.
