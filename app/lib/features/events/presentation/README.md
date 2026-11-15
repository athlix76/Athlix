# events/presentation

Screens: `events_screen.dart`, `event_detail_screen.dart` ("Who can join", NOT_ELIGIBLE sheet on register /
contact), `event_form_screen.dart` (with `event_eligibility_section.dart`), `my_events_screen.dart`.
Widgets: `event_card.dart` (eligibility chip, distance, greyed card with "Verify now"), `event_filters_sheet.dart`
("Organised by"), `event_discovery_bar.dart` ("Show all events" toggle, "Near me" radius chips).

## Changing it safely

Radius choices are capped by `event.max_search_radius_km`. Eligibility UI is hidden when `location.enabled`
is false. The eligibility rule is locked once an event is published.
