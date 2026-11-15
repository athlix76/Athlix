# core/theme

`app_theme.dart`: the single Material 3 dark theme (navy background `#031727`, cards `#0A1424`, fields
`#0D1D34`, cyan accent `#00D4EC`), taken from the approved login design. `app.dart` forces
`ThemeMode.dark`; `AppTheme.light()` returns the same theme. Change colours here only (the palette
constants at the top of the class), never hard-code colours in screens.
