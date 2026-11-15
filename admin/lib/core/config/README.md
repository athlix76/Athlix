# lib/core/config

Build-time settings from `--dart-define` (`API_BASE_URL`, `IDLE_TIMEOUT_MINUTES`).

| File | What it is |
|---|---|
| `env.dart` | Build-time settings, passed with `--dart-define`. |

## Changing it safely

- Only transport and session-safety values belong here. Business numbers (fees, limits, prices) always come from the backend.
