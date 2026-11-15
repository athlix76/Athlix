# docs/

Contracts first: code follows these files. When they disagree, fix the code or update the
doc **and** the code in the same change.

| File | What it is | Who must read it |
|---|---|---|
| [api.md](api.md) | Every HTTP endpoint, JSON shape, error code, WebSocket frame | backend, app, admin |
| [architecture.md](architecture.md) | Modules, table ownership, domain events, jobs, deployment | backend |
| [data-model.md](data-model.md) | Every table, generated from `backend/migrations` | backend |
| [config-system.md](config-system.md) | Every runtime setting, fee formula, premium benefits, verification gates | everyone |
| [security.md](security.md) | Hard rules, secrets, PII inventory, minor safety, admin permissions | everyone |
| [aws-setup.md](aws-setup.md) | Step-by-step AWS account → running production | operators |
| [flows/](flows/README.md) | Sequence diagrams of the important journeys (mermaid) | everyone |
| [business/](business/README.md) | Product overview, revenue model, compliance, costs, launch checklist | founders |

Diagrams use mermaid (renders on GitHub and in VS Code with a mermaid extension).
