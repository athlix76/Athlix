# infra/bootstrap

One-time setup per AWS account, applied before `envs/dev` or `envs/prod`.

## What it creates

- `athlix-tfstate-<account_id>`: S3 bucket for dev and prod state (versioned, AES256,
  public access blocked, TLS-only policy, old versions expire after 90 days,
  `prevent_destroy`).
- `athlix-tfstate-lock`: DynamoDB lock table (`LockID`, on-demand, point-in-time recovery).
- `athlix-apigateway-cloudwatch`: the account-level API Gateway CloudWatch role, needed
  for WebSocket stage logging (one per account and region).
- Optional, when `github_repo` is set: the GitHub OIDC provider and the role
  `athlix-github-terraform-plan`, which only pull requests of that repository can assume.
  It has `ReadOnlyAccess`, reads dev state, takes the lock, and may read only the dev db
  secret (plan refreshes it). Explicit denies stop it from reading media objects, queue
  messages, log contents, prod state and any other secret value.

## How to apply

```bash
cd infra/bootstrap
terraform init
terraform apply -var 'github_repo=<owner>/<repo>'   # omit the -var to skip the CI role
terraform output
```

Put `state_bucket` and `lock_table` into `envs/<env>/backend.hcl`, and store
`plan_role_arn` as the GitHub Actions secret `AWS_PLAN_ROLE_ARN`.

If the account already has a `token.actions.githubusercontent.com` provider, add
`-var create_github_oidc_provider=false`.

## State

This root uses **local state** (`infra/bootstrap/terraform.tfstate`, gitignored): it
creates the bucket the other roots store state in. It holds no secret. Keep a copy
somewhere safe (a password manager attachment is fine). If it is lost, the resources
still work; re-import them with `terraform import` before changing anything here.

## Changing it safely

The bucket and lock table have `prevent_destroy`; a plan that wants to replace them is
wrong, so stop. Changes to the plan role's policy affect only CI. Run `terraform plan`
and read it before every apply.
