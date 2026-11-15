# cmd/api

The HTTP API. `config.Load()` → `app.Bootstrap()` → `modules.All()` → `app.NewRouter()`.

- In AWS (`AWS_LAMBDA_FUNCTION_NAME` set): `lambda.Start(ginadapter.NewV2(r).ProxyWithContext)`
  behind an API Gateway **HTTP API** (payload format 2.0).
- Locally: listens on `HTTP_ADDR` (default `:8080`); events run in-process
  (`app.RegisterEventHandlers`), media is stored on disk (`LOCAL_STORAGE_DIR`) and the
  `chat` module mounts the local WebSocket at `/v1/chat/ws`.

Do not add routes here: modules register their own in `Routes()`.
