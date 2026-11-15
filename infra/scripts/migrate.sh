#!/usr/bin/env bash
# Invokes the migrate Lambda of an environment by hand and prints its result. Normally not
# needed: terraform apply runs it whenever the migrate code changes. Use it after a failed
# apply was fixed, or with run_migrations_on_apply = false.
#
# Usage: infra/scripts/migrate.sh <dev|prod>
# Needs: aws CLI v2 and an AWS SSO session (AWS_PROFILE) for the target account.
set -euo pipefail

env_name="${1:-}"
case "$env_name" in
  dev | prod) ;;
  *)
    echo "usage: $0 <dev|prod>" >&2
    exit 2
    ;;
esac

fn="athlix-$env_name-migrate"
out="$(mktemp)"
trap 'rm -f -- "$out"' EXIT

aws lambda invoke \
  --function-name "$fn" \
  --cli-binary-format raw-in-base64-out \
  --payload '{"trigger":"manual"}' \
  "$out" >/dev/null

# The response is the handler's summary string or an error object (no secrets in either).
cat "$out"
echo
if grep -q '"errorMessage"' "$out"; then
  echo "Migration failed; see CloudWatch log group /aws/lambda/$fn" >&2
  exit 1
fi
