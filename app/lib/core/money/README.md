# core/money

`money.dart`: `formatPaise`, `formatEntryFee`, `formatBp`, `parseRupeesToPaise`, `paiseToRupeeInput`.
All amounts are integer paise; percentages are basis points. No floating point anywhere.

## Changing it safely

Never convert money through `double`. Keep `test/money_test.dart` green.
