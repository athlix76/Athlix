# features/ads

Sponsored cards shown in fixed places of the app. Targeting is **contextual only**
(placement, sport, state); the app sends no personal data for ads.

## Use

```dart
AdSlot(placement: AdPlacement.eventsList, sportId: filters.sportId, state: filters.state)
```

`AdSlot` shows the first ad returned, records one impression per ad shown, and records a click
before opening `targetUrl` in the browser (http/https only). It renders nothing when the list is
empty (ads disabled, minor user) or the request fails.

## Endpoints

- `GET /ads?placement&sportId&state`
- `POST /ads/:id/impression`, `POST /ads/:id/click`

## Folders

- `data/`: `Ad` model, `AdQuery` record, `AdApi`
- `application/`: `adsProvider` (family by `AdQuery`, errors become `[]`)
- `presentation/`: `AdSlot`, `AdPlacement`

## Changing it safely

- A new placement must exist on the server first; then add it to `AdPlacement`.
- Always show `Ad.label` ("Sponsored"). Do not remove the disclosure.
- Do not add user ids, age or location beyond `state` to the query.
