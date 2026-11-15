# modules/monitoring

Alarms, an email topic and a budget for one environment.

## What it creates

- SNS topic `athlix-<env>-alerts` with email subscriptions (each address must click the
  confirmation link). Not KMS-encrypted: alarm text contains only names and numbers,
  and the AWS-managed SNS key cannot be used by CloudWatch alarms.
- Alarms: Lambda errors (>= 5 in 5 min) and throttles (>= 1) per function, HTTP API 5xx,
  RDS CPU > 80% for 15 min, RDS free storage < 2 GB, Redis memory > 80% per node, any
  message in the DLQ, oldest queued message > 10 min.
- AWS Budgets monthly cost budget (when `monthly_budget_usd > 0`): email at 80% actual and
  100% forecast. With `budget_env_tag_filter = true` it counts only `Environment=<env>`
  (activate the `Environment` cost allocation tag in Billing first).

## Outputs

`alerts_topic_arn`.

## Changing it safely

Thresholds are variables; tune them rather than deleting alarms. Alarm names include
the environment, so dev and prod never collide.
