# infra/envs

One Terraform root per environment. Each is thin: provider, backend, and a single call to
`modules/stack` with that environment's sizes.

| Folder | State key | Account |
|---|---|---|
| `dev/` | `athlix/dev/terraform.tfstate` | dev (may share the prod account at first) |
| `prod/` | `athlix/prod/terraform.tfstate` | prod |

Both store state in the bucket and lock table created by `infra/bootstrap`.

## Changing it safely

A new setting goes into `modules/stack/variables.tf` with a safe default, then is set
in the env's `main.tf` (or `variables.tf` + tfvars when it is per-operator). Do not add
resources directly to an env root; dev and prod would drift apart.
