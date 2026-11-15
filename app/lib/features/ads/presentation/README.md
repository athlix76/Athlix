# ads/presentation

- `ad_slot.dart`: `AdSlot` widget and `AdPlacement` codes.

Changing it safely: the impression set lives in widget state, so one slot records each ad
once. Keep the click recorded before the browser opens, and keep the http/https check on
`targetUrl`.
