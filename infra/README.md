# infra

Terraform for the Athlix AWS environments (region `ap-south-1`). The step-by-step guide
for a first deploy is [docs/aws-setup.md](../docs/aws-setup.md); this file covers the layout
and the rules for changing it.

## Layout

```
infra/
  bootstrap/        one-time per account: state bucket, lock table, API Gateway log role, GitHub OIDC plan role
  envs/
    dev/            dev root: smallest sizes, calls modules/stack
    prod/           prod root: sized for 10k to 1 lakh users, reasoning in main.tf comments
  modules/
    stack/          the whole environment; wires every module below
    network/        VPC, 2 AZs, one NAT, VPC endpoints (S3, Secrets Manager, SQS), security groups
    rds/            MySQL 8.0, writes the athlix/<env>/db secret value
    redis/          ElastiCache Redis 7
    lambda/         one Go Lambda (arm64, provided.al2023) with its log group and least-privilege role
    apigw_http/     HTTP API (payload v2) in front of the api Lambda
    apigw_ws/       WebSocket API in front of the ws Lambda, ManageConnections grants
    s3_cdn/         private media bucket, optional CloudFront for public prefixes
    secrets/        empty Secrets Manager containers under athlix/<env>/
    sqs/            events queue, DLQ, worker event source mapping
    scheduler/      one EventBridge Scheduler schedule per worker job (IST)
    monitoring/     alarms, SNS email topic, AWS Budgets
    ops_tunnel/     optional t4g.nano for SSM port-forwarding (admincli, one-off queries)
  scripts/          deploy.sh, migrate.sh, tunnel.sh
```

What runs where:

| Piece | AWS resource | Notes |
|---|---|---|
| `cmd/api` | Lambda `athlix-<env>-api` (512 MB, 29 s) | behind the HTTP API `$default` route |
| `cmd/ws` | Lambda `athlix-<env>-ws` (256 MB, 29 s) | `$connect`, `$disconnect`, `$default`, `sendMessage`, `typing`, `markRead`, `ping` |
| `cmd/worker` | Lambda `athlix-<env>-worker` (512 MB, 120 s) | SQS batches of 10 with partial failures, DLQ after 5 tries; Scheduler sends `{"job":"<name>"}` |
| `cmd/migrate` | Lambda `athlix-<env>-migrate` (256 MB, 300 s) | invoked by `terraform apply` when its zip changes, before the other three update |

The zips come from `make -C backend build-lambda` (`backend/dist/{api,ws,worker,migrate}.zip`).
Terraform reads them at plan time, so build first.

## Conventions

- Names: `athlix-<env>-<thing>`; secrets `athlix/<env>/<short-name>`.
- Tags: `Project`, `Environment`, `ManagedBy` come from the provider `default_tags`.
- Providers: roots pin `aws ~> 5.70`, `random ~> 3.6`, Terraform `>= 1.6`; modules accept
  any 5.x from 5.70.
- Environment roots stay thin. A difference between dev and prod is a variable of
  `modules/stack` set in `envs/<env>/main.tf`, never a copy of resources.
- No secret value is ever a Terraform variable, tfvars entry, output or Lambda environment
  variable. Lambdas get `SECRET_PREFIX` and read values from Secrets Manager.
- The `lambda` module rejects environment variable names outside its allowlist and IAM
  statements with `"*"` resources. Extend the allowlist only for non-secret settings.
- Every IAM grant names exact ARNs: each function reads only the secrets listed for it in
  `function_secrets` (`modules/stack/variables.tf`).

## State holds the db password

The rds module generates the MySQL master password with `random_password` and writes it
to the `athlix/<env>/db` secret. It is never an output, but Terraform keeps it in state.
That is why the state bucket is private, encrypted, versioned, TLS-only and never
readable by CI for prod, and why access to state is treated like access to the database.
The RDS-managed master secret was not used because its name and JSON shape do not match
what the backend reads (`{username,password,host,port,dbname}` under `athlix/<env>/db`).
To rotate: `terraform apply -replace=module.stack.module.rds.random_password.master`; RDS
takes the new password, the secret gets a new version, and the Lambdas pick it up on
their next cold start (force one by re-applying or updating the functions).

## Changing things safely

1. `terraform fmt -recursive infra` and `terraform validate` in the env you touched.
2. Plan dev, read the plan. Watch for `must be replaced` on `aws_db_instance`,
   `aws_elasticache_replication_group`, the media bucket or any secret: those lose data.
3. Apply dev, check the app, then plan and apply prod with the same commit.
4. Pull requests touching `infra/**` get an automatic dev plan
   (`.github/workflows/terraform-plan.yml`).

Do not rename modules or resources without a `moved {}` block; Terraform would destroy
and recreate them. Do not run `terraform destroy` on prod: deletion protection and
`prevent_destroy` are there to stop exactly that.
