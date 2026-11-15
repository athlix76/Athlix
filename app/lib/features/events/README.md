# features/events

Browse, create and manage sports events. Athlix **never collects entry fees**: `entryFeePaise`
is information only, players pay the organiser directly.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `EventsScreen` | `/events` (tab) | search, filter sheet (incl. "Organised by"), discovery bar ("Show all events", "Near me" radius), cursor list, `events_list` ad slot, create button for organisers |
| `EventDetailScreen` | `/events/:id` | entry-fee disclaimer, register / withdraw, contact reveal (call / WhatsApp), group chat, report; owner actions: pay listing fee, edit, registrations, feature, cancel |
| `EventFormScreen` | `/events/new`, `/events/:id/edit` | create or edit; Eligibility section (category, type, scope, place, live preview); live listing-fee estimate (debounced); published events lock fee, slots, dates and eligibility |
| `MyEventsScreen` | `/my-events` | my organised events by status |

## Endpoints

- `POST /events`, `PUT /events/:id`, `GET /events/:id`
- `GET /events` (filters in `EventFilters.toQuery`), `GET /events/featured`, `GET /me/events?status`
- `GET /events/:id/contact`, `POST /events/:id/cancel`, `POST /events/:id/feature`
- `GET /events/eligibility/preview` (form preview line)
- `GET /listing/quote` (estimate in the form, via `listing_payment/data`)
- `GET /events/:id/group` (via `chat/data`)

## Rules

- Field names in `data/event_models.dart` match docs/api.md exactly.
- Limits (max slots, free events allowed, min hours before start) come from public config
  (`event.*`); the server validates again. Nothing is hard-coded.
- Editing a published event sends only `EventEnums.publishedEditable`.
- The full contact phone from `/contact` is shown once in a sheet and never stored or logged.
- `EventInput.toString()` and `EventContact.toString()` redact, so they are safe in debug output.

## Location eligibility

- Cards and detail render `myEligibility` through `location_identity/application/eligibility_view.dart`:
  chip (`tag`), greyed card with reason and "Verify now" / "Renew now" / "In review" when not eligible.
- Ineligible events appear only with `showAll=true`. `NOT_ELIGIBLE` on register or contact opens the shared sheet.
- "Near me" sends `lat`, `lng`, `radiusKm`, `sort=distance`; the position is memory only, never stored or logged.
- Everything above is hidden when `location.enabled` is false.

## Changing it safely

Add a field: model `fromJson` → `EventInput.toJson` → form → detail. If it is editable after
publishing, add it to `EventEnums.publishedEditable` only if the backend allows it.
