# profile/data

- `profile_models.dart`: `Profile`, `ProfileSport`, `OrganiserInfo`, `ProfileUpdate`,
  `ProfileSportInput`, `ProfileViewer`, `StatEntry`, `StatsResult`, `StatInput`, `Achievement`,
  `AchievementInput`, plus `ProfileVisibility` and `AchievementLevel` value lists.
- `profile_api.dart`: `ProfileApi` and `profileApiProvider`.

Changing it safely: field names match docs/api.md exactly. `ProfileUpdate.toJson` sends only
non-null fields, so leaving a field null means "do not change it".
