# modules/apigw_http

HTTP API (API Gateway v2) in front of the api Lambda.

## What it creates

- HTTP API with one `$default` route to a Lambda proxy integration (payload format 2.0,
  30 s timeout). CORS is handled by the backend, not here.
- `$default` stage with auto-deploy, stage-wide throttling and JSON access logs to
  `/aws/apigateway/<name>`. The log format has request id, time, IP, user agent, method,
  path, status, sizes and latencies only: no headers (so no `Authorization`) and no query
  string.
- Permission for API Gateway to invoke the function.
- Optional custom domain (regional, TLS 1.2) and API mapping when `domain_name` and
  `certificate_arn` are set.

## Outputs

`api_id`, `execute_api_url`, `base_url` (custom domain or execute-api URL, no `/v1`),
`custom_domain_target` (CNAME target), `access_log_group_name`.

## Changing it safely

Never add `$request.header.*` or `$request.querystring` fields to the access log format.
Adding or removing the custom domain is safe; clients must switch URLs.
