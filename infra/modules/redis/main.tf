resource "aws_elasticache_subnet_group" "this" {
  name        = var.name
  description = "Private subnets for ${var.name} Redis"
  subnet_ids  = var.private_subnet_ids
}

resource "aws_security_group" "redis" {
  name        = "${var.name}-redis"
  description = "Redis: reachable only from the Lambda (and optional ops tunnel) security groups"
  vpc_id      = var.vpc_id
  tags        = { Name = "${var.name}-redis" }
}

resource "aws_vpc_security_group_ingress_rule" "redis" {
  count = length(var.allowed_security_group_ids)

  security_group_id            = aws_security_group.redis.id
  description                  = "Redis from an allowed security group"
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
  referenced_security_group_id = var.allowed_security_group_ids[count.index]
}

# Redis holds only short-lived, non-secret state (hashed OTPs, rate-limit counters,
# WebSocket connection ids, ws tickets, OAuth state). Protection is layered:
# private subnets + a security group open only to the Lambda SG, encryption at rest, and
# in-transit encryption in "required" mode (the backend client dials TLS outside local).
# An AUTH token is deliberately not managed here: it would land in Terraform state. If one
# is ever wanted, create it in Secrets Manager outside Terraform (cache.Options.Password).
resource "aws_elasticache_replication_group" "this" {
  replication_group_id = var.name
  description          = "Athlix ${var.name} Redis"

  engine               = "redis"
  engine_version       = var.engine_version
  node_type            = var.node_type
  num_cache_clusters   = var.num_cache_clusters
  parameter_group_name = "default.redis7"
  port                 = 6379

  automatic_failover_enabled = var.num_cache_clusters > 1
  multi_az_enabled           = var.num_cache_clusters > 1

  subnet_group_name  = aws_elasticache_subnet_group.this.name
  security_group_ids = [aws_security_group.redis.id]

  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  transit_encryption_mode    = var.transit_encryption_mode

  snapshot_retention_limit   = var.snapshot_retention_days
  snapshot_window            = "18:30-19:30"
  maintenance_window         = "sun:20:00-sun:21:00"
  auto_minor_version_upgrade = true
  apply_immediately          = var.apply_immediately

  tags = { Name = var.name }
}
