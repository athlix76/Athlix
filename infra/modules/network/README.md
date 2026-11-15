# modules/network

VPC for one environment across two availability zones.

## What it creates

- VPC (default `10.20.0.0/16`, DNS on), the default security group locked down.
- Two public subnets (internet gateway) and two private subnets (Lambdas, RDS, Redis).
- **One NAT gateway** in the first public subnet, shared by both private subnets. This is
  a cost choice: a NAT is about $41/month each before data. The trade-off is that if the
  NAT's AZ fails, outbound internet calls (partners) stop until it recovers; AWS services
  reached through endpoints keep working.
- VPC endpoints: S3 (gateway, free), Secrets Manager and SQS (interface, about $8/month
  per AZ each; `interface_endpoint_subnet_count` 1 in dev, 2 in prod).
- Security groups: `lambda` and `ops` (egress only, no ingress), `endpoints` (443 from the VPC).

## Outputs

`vpc_id`, `vpc_cidr`, `private_subnet_ids`, `public_subnet_ids`,
`lambda_security_group_id`, `ops_security_group_id`, `nat_public_ip` (give it to partners
that allowlist IPs), `nat_gateway_id`, `endpoint_ids`.

## Changing it safely

Changing `vpc_cidr` or the subnet layout replaces the VPC and everything in it (RDS,
Redis, Lambda ENIs): treat it as a rebuild. Adding a second NAT is safe: add a NAT per
public subnet and one private route table per AZ.
