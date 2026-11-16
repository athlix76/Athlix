# core/widgets

Reusable widgets with no feature logic: `ErrorView` / `errorMessage` / snacks, `AsyncValueView`, `EmptyView`,
`CursorListView` (cursor pagination), `BusyButton`, pickers and `confirm` dialog, `SimpleMarkdown` (policy
text), `SportDropdown` / `EnumDropdown`, `SectionHeader` / `InfoRow`, `UserAvatar` / `SignedImage`, `UserTile`,
`AthlixLogo` (the glass icon from `assets/brand/athlix_icon.png`, see `app/tool/brand`), `AthlixWordmark`
(mark over "ATHLIX", `assets/brand/athlix_wordmark.png`), `AthlixMarkTitle` (mark beside "ATHLIX",
`assets/brand/athlix_mark.png`), and the shared look in `page_hero.dart`: `PageHero` (tab header:
eyebrow, title, subtitle), `HeroIconButton`, `HeroButton`, `RoundIconButton`, `GlowFab`, `PillToggle`
("Following | Latest"), `PillChips` and `searchFieldDecoration`. `CursorListView.endText` shows a line
under the last page (e.g. "You're all caught up").

## Changing it safely

`errorMessage` must only show API messages or a generic sentence, never `toString()` of an exception.
Signed image URLs are short-lived; do not cache them across sessions.
