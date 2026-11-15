# modules/apigw_ws

WebSocket API in front of the ws Lambda.

## What it creates

- WebSocket API with route selection `$request.body.action`.
- Routes `$connect`, `$disconnect`, `$default`, `sendMessage`, `typing`, `markRead`,
  `ping`, all to one Lambda proxy integration. Authentication is done by the ws Lambda
  on `$connect` with the `?ticket=` value; routes are `NONE` at the gateway.
- Deployment (redeployed when routes change) and a stage (default `live`) with
  throttling, ERROR-level execution logs, no data trace, optional access logs.
- Permission for API Gateway to invoke the ws function.
- `execute-api:ManageConnections` on this API's stage for each role in
  `manage_connections_role_names` (api, ws, worker), so any of them can push to clients.
- Optional custom domain and mapping.

## Outputs

`management_url` (https://..., `WS_MANAGEMENT_URL`), `public_url` (wss://...,
`WS_PUBLIC_URL`), `api_id`, `execution_arn`, `custom_domain_target`.

## Changing it safely

Execution and access logging need the account-level CloudWatch role from
`infra/bootstrap`. Data trace stays off (it would log message bodies). A new client
action needs a route here and a handler in `cmd/ws`.
