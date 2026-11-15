# cmd/migrate

Applies `backend/migrations/*.sql` (embedded via `migrations/embed.go`) in file-name order,
each once, tracked in `schema_migrations`.

- Locally: `make migrate` (needs `DB_DSN`, e.g. the docker compose MySQL).
- AWS: the `athlix-<env>-migrate` Lambda reads the `db` secret; the deploy workflow invokes
  it before updating the api/ws/worker Lambdas.

Migrations are forward-only. Never edit one that already ran in a shared environment.
