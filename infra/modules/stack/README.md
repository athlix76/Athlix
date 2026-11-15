# modules/stack

The whole Athlix environment, wired in one place so dev and prod differ only in the
variables they pass.

## What it creates

network, secrets, rds, redis, media bucket (`athlix-<env>-media-<account_id>`), the four
Lambdas (`athlix-<env>-{api,ws,worker,migrate}`), the HTTP and WebSocket APIs, the events
queue with its DLQ and worker mapping, the Scheduler jobs, monitoring and, when
`enable_ops_tunnel = true`, the ops tunnel instance.

## Order on apply

1. network, secrets, rds (writes the `db` secret value), redis, media, sqs.
2. The migrate Lambda, then `aws_lambda_invocation.migrate`, which runs the migrations.
   It re-runs when the migrate zip or the db secret version changes.
3. api, ws and worker (`depends_on` the invocation): if a migration fails, the apply
   stops and the old code keeps running.

## Per-function access

| Function | Secrets (`function_secrets`) | Other IAM |
|---|---|---|
| api | all ten | media Put/Get/Delete + ListBucket, SQS SendMessage, ManageConnections |
| ws | db, jwt, pii-keys | media GetObject, SQS SendMessage, ManageConnections |
| worker | db, jwt, pii-keys, msg91, fcm, razorpay, google-play, app-store | media Put/Get/Delete + ListBucket (covers `media.cleanup_pending` and `location.purge_proofs` on `location_proof/`), SQS consume + send, ManageConnections |
| migrate | db | none |

Environment variables (non-secret only): `ENV`, `SECRET_PREFIX`, `REDIS_ADDR`,
`MEDIA_BUCKET`, `LOG_LEVEL` for all four; api, ws and worker also get
`EVENTS_QUEUE_URL`, `WS_MANAGEMENT_URL`, `WS_PUBLIC_URL`, `PUBLIC_API_BASE_URL`,
`APP_DEEP_LINK_BASE`, `ADMIN_ORIGINS`. migrate needs `REDIS_ADDR` and `MEDIA_BUCKET` only
because `config.Load()` validates them outside local.

## Outputs

`api_base_url`, `app_api_url` (`<base>/v1`), `ws_public_url`, `ws_management_url`,
custom-domain CNAME targets, `media_bucket`, `media_bucket_origin` (for the admin web CSP), `cdn_domain_name`, queue URLs,
`secret_names`, `secret_prefix`, `nat_public_ip`, `rds_address`, `redis_address`,
`ops_tunnel_instance_id`, `function_names`, `alerts_topic_arn`. None is a secret.

## Changing it safely

- A new secret: add it to `modules/secrets` and to the right lists in `function_secrets`.
- A new environment variable: add it to the `lambda` module allowlist and to
  `base_env` / `app_env`. Never a secret value.
- A new IAM need: add a statement local with exact ARNs and attach it to the function
  that needs it, not to all of them.
- Keep the migrate invocation ahead of the app functions; removing the `depends_on`
  lets new code run against an old schema.
