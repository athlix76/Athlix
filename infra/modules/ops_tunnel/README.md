# modules/ops_tunnel

An on-demand `t4g.nano` (Amazon Linux 2023, arm64) in a private subnet, used only as a
Session Manager hop so an operator can port-forward to RDS or Redis from a laptop. It
has no public IP, no SSH key and no inbound rule; IMDSv2 is required and the root disk
is encrypted. Its role has only `AmazonSSMManagedInstanceCore`: no Secrets Manager, S3 or
SQS access.

Created when `enable_ops_tunnel = true` in the env; use `infra/scripts/tunnel.sh`, then
set it back to false and apply to remove it (about $3/month if left running).

## Outputs

`instance_id`.

## Changing it safely

Do not add a public IP, key pair or ingress rule. The AMI is ignored after creation so
re-applies do not replace the instance; recreate it to pick up a newer AMI.
