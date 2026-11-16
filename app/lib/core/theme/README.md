# core/theme

`app_theme.dart`: the single dark Material 3 theme, from the approved home screenshot: deep navy
background `#070C1A` with soft cyan glows (`glowTop` / `glowBottom`, painted by `HomeShell`), dark
cards `#0D1626` with a faint border, fields and pill tracks `#111C2E`, cyan primary `#05CDE0` and the
`cyanGradient` (to `#19B9F3`) for the selected tab and the main action. Headings use Montserrat, body
text Manrope; both are bundled in `assets/fonts` (SIL OFL).

`app.dart` forces `ThemeMode.dark`; `AppTheme.light()` returns the same theme. Change colours here
only (the palette constants at the top of the class), never hard-code colours in screens.
