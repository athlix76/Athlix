# features/registrations

Player registration for events and the organiser view of registrations.

## Endpoints

- `POST /events/:id/registrations`
- `DELETE /events/:id/registrations/me`
- `GET /me/registrations`
- `GET /events/:id/registrations (?flagged=true)`
- `PATCH /events/:id/registrations/:regId`

## Rules

- Payment status is a note the organiser records; Athlix never takes the entry fee.
- Eligibility is checked by the server; `NOT_ELIGIBLE` opens the shared sheet.

## Changing it safely

Team size limits come from the event; the 30 member UI cap is only a fallback.
