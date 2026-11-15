# lib/core/format

Display and input helpers: paise and rupees, basis points, dates, and the line diff used by "review before save".

| File | What it is |
|---|---|
| `format.dart` | Display and input helpers for money (integer paise), basis points and times. |
| `line_diff.dart` | A minimal line diff (longest common subsequence) for "review before save". |

## Changing it safely

- Money is integer paise and percentages are basis points everywhere; never use doubles for money.
