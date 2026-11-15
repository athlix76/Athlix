# Nothing here is a secret. Secret VALUES are never outputs; only secret names.

output "api_base_url" {
  description = "HTTP API base URL (custom domain if set), without /v1. The backend's PUBLIC_API_BASE_URL."
  value       = module.http_api.base_url
}

output "app_api_url" {
  description = "What the app and admin web use as their API base."
  value       = "${module.http_api.base_url}/v1"
}

output "api_custom_domain_target" {
  description = "Point a CNAME for api_domain_name here (empty without a custom domain)."
  value       = module.http_api.custom_domain_target
}

output "ws_public_url" {
  description = "wss:// URL the app connects to (with ?ticket=...)."
  value       = module.ws_api.public_url
}

output "ws_custom_domain_target" {
  value = module.ws_api.custom_domain_target
}

output "ws_management_url" {
  description = "https:// connections endpoint used by api / ws / worker (WS_MANAGEMENT_URL)."
  value       = module.ws_api.management_url
}

output "media_bucket" {
  value = module.media.bucket_name
}

output "media_bucket_origin" {
  description = "https://<bucket>.s3.<region>.amazonaws.com: put it in the admin web CSP (connect-src, img-src) at deploy."
  value       = module.media.bucket_origin
}

output "cdn_domain_name" {
  value = module.media.cdn_domain_name
}

output "events_queue_url" {
  value = module.sqs.queue_url
}

output "events_dlq_url" {
  value = module.sqs.dlq_url
}

output "secret_names" {
  description = "Secrets Manager names to fill with put-secret-value (db is filled by Terraform)."
  value       = module.secrets.names
}

output "secret_prefix" {
  value = local.secret_prefix
}

output "nat_public_ip" {
  description = "Egress IP of every Lambda. Give it to partners that allowlist callers."
  value       = module.network.nat_public_ip
}

output "rds_address" {
  description = "Private hostname of the MySQL instance (not reachable from the internet)."
  value       = module.rds.address
}

output "redis_address" {
  description = "Private host:port of Redis (REDIS_ADDR)."
  value       = module.redis.address
}

output "ops_tunnel_instance_id" {
  description = "SSM target for scripts/tunnel.sh; empty when enable_ops_tunnel = false."
  value       = var.enable_ops_tunnel ? module.ops_tunnel[0].instance_id : ""
}

output "function_names" {
  value = local.fn
}

output "alerts_topic_arn" {
  value = module.monitoring.alerts_topic_arn
}
