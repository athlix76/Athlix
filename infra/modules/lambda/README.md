# modules/lambda

One Go Lambda function. Used four times by `stack` (api, ws, worker, migrate).

## What it creates

- `aws_lambda_function`: `provided.al2023`, `arm64`, handler `bootstrap`, code from
  `zip_path` (`backend/dist/<name>.zip`), VPC config, memory, timeout, reserved concurrency.
- Log group `/aws/lambda/<function_name>` with the given retention (created before the
  function so retention applies from the first log line).
- An execution role with one inline policy: write its own log group, manage VPC network
  interfaces, `secretsmanager:GetSecretValue` on exactly `secret_arns`, plus
  `policy_statements`.

Guard rails: `environment` keys must be in the allowlist in `variables.tf` (non-secret
settings only); `policy_statements` may not use `"*"` as a resource.

## Outputs

`function_name`, `arn`, `invoke_arn`, `source_code_hash`, `role_name`, `role_arn`,
`log_group_name`.

## Changing it safely

Adding a statement or a secret ARN is safe. A new environment variable needs an
allowlist entry here first; if a value is sensitive, it belongs in Secrets Manager
instead. Renaming `function_name` replaces the function and its log group.
