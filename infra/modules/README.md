# infra/modules

Reusable Terraform modules. `stack/` composes all the others into one environment; the
env roots call only `stack/`. Each module folder has `versions.tf`, `main.tf`,
`variables.tf`, `outputs.tf` and a README describing its inputs and outputs.

## Changing it safely

Modules take names and ids as inputs and never read secret values. Keep for_each keys
known at plan time (use names, not computed ids). Run `terraform validate` in both env
roots after changing a module, since both use it.
