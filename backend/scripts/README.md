# backend/scripts

Developer scripts that are not part of any binary. Nothing here is deployed.

| Folder | What |
|---|---|
| [smoke/](smoke/README.md) | End-to-end smoke test against a local API (onboarding, organiser approval, listing fee from config, config change, location eligibility, ownership) |

Rules: scripts run against a **local** stack only, use the public HTTP API (no direct DB writes),
never print tokens, passwords, OTPs or PII, and use only the Python standard library (or Go
from this module) so they need no install.
