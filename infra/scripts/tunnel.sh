#!/usr/bin/env bash
# Opens an SSM Session Manager port-forward from this machine to the private RDS or Redis
# endpoint of an environment, through the ops_tunnel instance. No SSH, no public IP, no
# inbound rule: the session runs over your AWS SSO credentials.
#
# Usage:
#   infra/scripts/tunnel.sh <dev|prod> mysql [local_port]   # default local port 3306
#   infra/scripts/tunnel.sh <dev|prod> redis [local_port]   # default local port 6379
#
# Needs: aws CLI v2, the Session Manager plugin, terraform, an SSO login
# (aws sso login --profile <profile>, then export AWS_PROFILE=<profile>), and
# enable_ops_tunnel = true applied in that environment. Leave the command running; press
# Ctrl+C to close the tunnel. See docs/aws-setup.md ("Create the first admin").
set -euo pipefail

env_name="${1:-}"
target="${2:-}"

case "$env_name" in
  dev | prod) ;;
  *)
    echo "usage: $0 <dev|prod> <mysql|redis> [local_port]" >&2
    exit 2
    ;;
esac

here="$(cd "$(dirname "$0")" && pwd)"
env_dir="$here/../envs/$env_name"

tf_out() {
  terraform -chdir="$env_dir" output -raw "$1"
}

instance_id="$(tf_out ops_tunnel_instance_id)"
if [ -z "$instance_id" ]; then
  echo "No ops tunnel instance in $env_name. Set enable_ops_tunnel = true in terraform.tfvars and apply." >&2
  exit 1
fi

case "$target" in
  mysql)
    remote_host="$(tf_out rds_address)"
    remote_port=3306
    ;;
  redis)
    redis_addr="$(tf_out redis_address)"
    remote_host="${redis_addr%:*}"
    remote_port="${redis_addr##*:}"
    ;;
  *)
    echo "usage: $0 <dev|prod> <mysql|redis> [local_port]" >&2
    exit 2
    ;;
esac

local_port="${3:-$remote_port}"
params="{\"host\":[\"$remote_host\"],\"portNumber\":[\"$remote_port\"],\"localPortNumber\":[\"$local_port\"]}"

echo "Forwarding 127.0.0.1:$local_port -> $target ($env_name) via $instance_id. Ctrl+C to stop."
exec aws ssm start-session \
  --target "$instance_id" \
  --document-name AWS-StartPortForwardingSessionToRemoteHost \
  --parameters "$params"
