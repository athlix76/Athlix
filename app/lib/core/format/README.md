# core/format

Display helpers: `formatDateTime`, `formatDate`, `formatTime`, `timeAgo`, `parseDateOnly`, `humanize`.
API times are UTC; these show device-local time.

## Changing it safely

Formatting only. Do not parse wire dates here (use `core/json`), and do not format money here (use `core/money`).
