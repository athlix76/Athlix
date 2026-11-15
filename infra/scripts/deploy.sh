#!/usr/bin/env bash
# The deploy routine for one environment: build the four Lambda zips, show the plan, and
# apply it after you confirm. During apply Terraform invokes the migrate Lambda first
# (only when its code changed) and updates api / ws / worker only if migrations succeed.
#
# Usage: infra/scripts/deploy.sh <dev|prod>
# Needs: Go, make, zip, terraform, an AWS SSO session (AWS_PROFILE) for the target account,
# and infra/envs/<env>/backend.hcl + terraform.tfvars in place (terraform init done once).
set -euo pipefail

env_name="${1:-}"
case "$env_name" in
  dev | prod) ;;
  *)
    echo "usage: $0 <dev|prod>" >&2
    exit 2
    ;;
esac

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
env_dir="$repo/infra/envs/$env_name"
plan_file="$env_dir/deploy.tfplan"

echo "==> Building Lambda zips (backend/dist)"
make -C "$repo/backend" build-lambda

echo "==> Planning $env_name"
terraform -chdir="$env_dir" plan -out="$plan_file"

read -r -p "Apply this plan to $env_name? Type the environment name to confirm: " answer
if [ "$answer" != "$env_name" ]; then
  echo "Not applied."
  rm -f -- "$plan_file"
  exit 1
fi

terraform -chdir="$env_dir" apply "$plan_file"
rm -f -- "$plan_file"

echo "==> Done. App API: $(terraform -chdir="$env_dir" output -raw app_api_url)"
