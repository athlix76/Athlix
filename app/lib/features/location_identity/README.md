# features/location_identity

Location eligibility: a player's native place and residence identities, and the shared view logic for event eligibility.

## Endpoints

- `GET /geo/states`
- `GET /geo/districts?stateId`
- `GET /me/location-identities`
- `POST /me/location-identities`
- `DELETE /me/location-identities/:id`
- `POST /media/uploads (purpose location_proof)`

## Rules

- The server decides eligibility; the app only renders `myEligibility` and `NOT_ELIGIBLE` `fields.reason` / `fields.action`.
- Proofs are uploaded through `core/media`; the app never sees S3 keys.
- Device location is not used here and is never stored or logged.
- Hidden entirely when `location.enabled` is false.

## Changing it safely

New reasons or actions: add the constant, the sentence / label / route in `eligibility_view.dart`, and a case in `test/eligibility_view_test.dart`.
