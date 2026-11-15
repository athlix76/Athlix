# backend/cmd

One folder per binary. Each `main.go` only wires things together; logic lives in `internal/`.

| Binary | Runs as | What it does |
|---|---|---|
| [api](api/README.md) | Lambda behind API Gateway HTTP API, or `make run` locally | every REST route (+ local WebSocket and in-process events when `ENV=local`) |
| [ws](ws/README.md) | Lambda behind API Gateway WebSocket API | `$connect`, `$disconnect`, `sendMessage`, `typing`, `markRead`, `ping` |
| [worker](worker/README.md) | Lambda on SQS `athlix-<env>-events` + EventBridge Scheduler | domain event handlers and scheduled jobs |
| [migrate](migrate/README.md) | Lambda invoked by the deploy workflow, or `make migrate` | applies `backend/migrations` |
| [admincli](admincli/README.md) | operator laptop (`go run`) | create the first admin, reset credentials |
| [devsecrets](devsecrets/README.md) | developer laptop (`make dev-secrets`) | random local keys in `.secrets.local.json` |
