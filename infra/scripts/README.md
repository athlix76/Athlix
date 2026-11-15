# infra/scripts

Small operator scripts. They read Terraform outputs and use your AWS SSO session
(`AWS_PROFILE`); none of them holds, prints or stores a secret.

| Script | What it does |
|---|---|
| `deploy.sh <dev\|prod>` | `make -C backend build-lambda`, `terraform plan -out`, asks you to type the env name, then applies. Migrations run inside the apply. |
| `migrate.sh <dev\|prod>` | Invokes `athlix-<env>-migrate` by hand and prints its result. Only needed after a failed apply or with `run_migrations_on_apply = false`. |
| `tunnel.sh <dev\|prod> <mysql\|redis> [local_port]` | SSM Session Manager port-forward to the private RDS or Redis endpoint through the `ops_tunnel` instance (set `enable_ops_tunnel = true` first). |

Requirements: bash, aws CLI v2, the Session Manager plugin (for `tunnel.sh`),
terraform, and Go/make/zip (for `deploy.sh`).

## Changing them safely

Keep them free of secret values and of `set -x` (it would echo commands). Run
`bash -n <script>` after editing; they use `set -euo pipefail`, so a failed step stops
the script.
