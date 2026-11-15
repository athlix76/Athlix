output "replication_group_id" {
  value = aws_elasticache_replication_group.this.id
}

output "primary_endpoint" {
  description = "Primary endpoint host (private DNS)."
  value       = aws_elasticache_replication_group.this.primary_endpoint_address
}

output "port" {
  value = aws_elasticache_replication_group.this.port
}

output "address" {
  description = "host:port, the value of REDIS_ADDR."
  value       = "${aws_elasticache_replication_group.this.primary_endpoint_address}:${aws_elasticache_replication_group.this.port}"
}

output "node_ids" {
  description = "Cache cluster (node) ids for per-node alarms: <group>-001, <group>-002, ... Built from var.name so they are known at plan time."
  value       = [for i in range(var.num_cache_clusters) : format("%s-%03d", var.name, i + 1)]
}

output "security_group_id" {
  value = aws_security_group.redis.id
}
