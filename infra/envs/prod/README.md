# infra/envs/prod

The prod environment root, sized for 10,000 to 1,00,000 (1 lakh) users. The reasoning for
every size is in the comments of `main.tf`. In short:

| Area | Prod | Dev |
|---|---|---|
| MySQL | `db.t4g.medium`, Multi-AZ, 50 GB gp3 (autoscale to 500), 14-day backups, deletion protection, final snapshot, Performance Insights | `db.t4g.micro`, single-AZ, 1-day backups |
| Redis | 2 x `cache.t4g.small`, automatic failover | 1 x `cache.t4g.micro` |
| Endpoints | ENIs in both AZs | one AZ |
| Lambda | reserved concurrency api 100 / ws 50 / worker 10; SQS max concurrency 5 | unreserved; SQS max concurrency 2 |
| API Gateway throttling | HTTP 500 rps / 1000 burst; WS 200 / 400 | 50 / 100 |
| Logs | 30 days | 14 days |
| Secrets | 30-day recovery window | deleted immediately |
| Media | versioned, never force-destroyed | unversioned, force-destroy |

Reserved concurrency needs the account's Lambda concurrency quota raised to 1,000
(Service Quotas, "Concurrent executions"). Until it is granted, set the three
`*_reserved_concurrency` variables to `-1` in `terraform.tfvars`.

## Files

Same as `envs/dev`: `versions.tf`, `main.tf`, `variables.tf`, `outputs.tf`,
`backend.hcl.example`, `terraform.tfvars.example`.

## Use

```bash
make -C backend build-lambda
cd infra/envs/prod
cp backend.hcl.example backend.hcl
cp terraform.tfvars.example terraform.tfvars
terraform init -backend-config=backend.hcl
terraform plan -out=tfplan      # read it; nothing stateful should be replaced
terraform apply tfplan
```

## Changing it safely

- Apply the same commit to dev first.
- Changing `db_instance_class` or Redis `node_type` causes a short failover (Multi-AZ);
  do it in a quiet hour.
- Never set `db_deletion_protection = false` or `media_force_destroy = true` here except
  as a deliberate, reviewed teardown (see docs/aws-setup.md, "Teardown").
