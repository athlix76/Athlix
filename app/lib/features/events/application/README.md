# events/application

`event_providers.dart`: list / detail / featured / my-events providers, `eventFiltersProvider`, and
`eligibilityPreviewProvider` (keyed by category + rule) for the form.

## Changing it safely

Providers are autoDispose; invalidate detail and list after a mutation.
