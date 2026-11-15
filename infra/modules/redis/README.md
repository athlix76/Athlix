# modules/redis

ElastiCache Redis 7 replication group for one environment.

## What it creates

- Subnet group, security group allowing 6379 only from the given security groups.
- Replication group: `num_cache_clusters` nodes (2+ enables automatic failover and
  Multi-AZ), encryption at rest, in-transit encryption in **`required` mode** (TLS only).

The backend Redis client (`internal/platform/cache`) dials TLS outside local
(`REDIS_TLS` defaults to `true`). `preferred` (TLS and plain text) is kept only as a
fallback for a migration window. No AUTH token: it would be a secret stored in Terraform
state, so access is limited by TLS, the private subnets and the security group. If one is
ever added, create it in Secrets Manager outside Terraform and pass it as
`cache.Options.Password`.

## Outputs

`address` (host:port, used as `REDIS_ADDR`), `primary_endpoint`, `port`, `node_ids` (for
per-node alarms, known at plan), `replication_group_id`, `security_group_id`.

## Changing it safely

`node_type` and `num_cache_clusters` change in place. Changing `name`, the subnet group
or turning in-transit encryption off replaces the group (cached data is lost, which is
acceptable but logs users out of anything stored there).
