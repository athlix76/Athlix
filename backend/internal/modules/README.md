# backend/internal/modules

One folder per feature. Ownership, events and jobs: [docs/architecture.md](../../../docs/architecture.md).
Endpoints: [docs/api.md](../../../docs/api.md).

## Adding a module

1. `internal/modules/<name>/` with `module.go`, `handler.go`, `service.go`, `repository.go`,
   `model.go`, `<name>_test.go`, `README.md`.
2. `New(d *app.Deps) *Module`, `Name()`, `Routes(r app.Routes)`; optional
   `EventHandlers()` and `Jobs()`.
3. Add one line to `registry.go`.
4. Document routes in docs/api.md, tables in a migration + docs/data-model.md, events and
   jobs in docs/architecture.md, config keys in docs/config-system.md.

## Module README template

```markdown
# <name>
What it does (2-3 lines).
## Routes      (table: method, path, group, permission; link to docs/api.md section)
## Tables      (owned / read)
## Events      (published / consumed)
## Jobs
## Config keys
## Rules       (the business rules a change must keep)
## Tests       (what is covered)
```
