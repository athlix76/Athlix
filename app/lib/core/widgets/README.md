# core/widgets

Reusable widgets with no feature logic: `ErrorView` / `errorMessage` / snacks, `AsyncValueView`, `EmptyView`,
`CursorListView` (cursor pagination), `BusyButton`, pickers and `confirm` dialog, `SimpleMarkdown` (policy
text), `SportDropdown` / `EnumDropdown`, `SectionHeader` / `InfoRow`, `UserAvatar` / `SignedImage`, `UserTile`,
`AthlixLogo` (the glass icon from `assets/brand/athlix_icon.png`, see `app/tool/brand`).

## Changing it safely

`errorMessage` must only show API messages or a generic sentence, never `toString()` of an exception.
Signed image URLs are short-lived; do not cache them across sessions.
