# infra/envs/dev

The dev environment root. It configures the provider (`ap-south-1`, default tags) and
calls `modules/stack` with the smallest sizes: `db.t4g.micro` single-AZ, one
`cache.t4g.micro`, one interface-endpoint ENI each, no reserved concurrency, 14-day logs,
media bucket and secrets that can be destroyed outright.

## Files

| File | Purpose |
|---|---|
| `versions.tf` | Terraform and provider versions, partial `backend "s3" {}` |
| `main.tf` | provider and the `stack` module call with dev sizes |
| `variables.tf` | what you set per person/account in `terraform.tfvars` |
| `outputs.tf` | URLs, bucket (and its origin for the admin web CSP), queue, secret names, hostnames for the tunnel (no secrets) |
| `backend.hcl.example` | copy to `backend.hcl`; state bucket, key, lock table |
| `terraform.tfvars.example` | copy to `terraform.tfvars` (gitignored); never put a secret in it |

## Use

```bash
make -C backend build-lambda          # zips must exist before plan
cd infra/envs/dev
cp backend.hcl.example backend.hcl    # fill in the bucket from infra/bootstrap
cp terraform.tfvars.example terraform.tfvars
terraform init -backend-config=backend.hcl
terraform plan -out=tfplan
terraform apply tfplan
```

Or use `infra/scripts/deploy.sh dev`. The full first-time walkthrough, including filling
secrets and creating the first admin, is in `docs/aws-setup.md`.

## Changing it safely

Change sizes here, structure in `modules/stack`. Dev may be torn down with
`terraform destroy` (the media bucket is emptied and secrets are deleted immediately).
