# backend/internal

| Folder | What |
|---|---|
| [app](app/README.md) | `Deps` (everything a module may use), `Bootstrap`, router + route groups, module interfaces |
| [platform](platform/README.md) | shared technical building blocks (no business rules) |
| [modules](modules/README.md) | feature modules, one folder each |
| [testutil](testutil/README.md) | test helpers: sqlmock DB, miniredis, static settings, test users |

Dependency direction: `modules → app → platform`. Platform never imports modules; modules
never import each other across agents (see docs/architecture.md).
