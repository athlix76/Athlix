# profile/presentation

- Screens: `profile_screen.dart` (also `ProfileBody`), `stats_screen.dart`,
  `achievements_screen.dart` (also `AchievementTile`), `edit_profile_screen.dart`,
  `profile_viewers_screen.dart`, `my_profile_screen.dart`.
- Shared parts: `profile_header.dart` (`ProfileHeader`), `stat_entry_sheet.dart`
  (`showStatEntrySheet`), `stat_format.dart` (type-based label / parse / format),
  `achievement_sheet.dart` (`showAchievementSheet`, certificate upload).

Changing it safely: keep stat handling keyed on `StatField.type`, never on a stat key.
`duration_ms` values are entered in seconds and sent as milliseconds.
