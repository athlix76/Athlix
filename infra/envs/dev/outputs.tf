# Plain pass-through of the stack outputs. None of them is a secret.

output "api_base_url" {
  description = "HTTP API base without /v1."
  value       = module.stack.api_base_url
}

output "app_api_url" {
  description = "API base for the app and admin web (<base>/v1)."
  value       = module.stack.app_api_url
}

output "api_custom_domain_target" {
  description = "CNAME target for api_domain_name."
  value       = module.stack.api_custom_domain_target
}

output "ws_public_url" {
  description = "wss:// URL for chat."
  value       = module.stack.ws_public_url
}

output "ws_custom_domain_target" {
  description = "CNAME target for ws_domain_name."
  value       = module.stack.ws_custom_domain_target
}

output "ws_management_url" {
  description = "WS_MANAGEMENT_URL used by the Lambdas."
  value       = module.stack.ws_management_url
}

output "media_bucket" {
  description = "Private media bucket."
  value       = module.stack.media_bucket
}

output "media_bucket_origin" {
  description = "Origin of presigned media URLs; the admin web deploy writes it into the CSP."
  value       = module.stack.media_bucket_origin
}

output "cdn_domain_name" {
  description = "CloudFront domain (empty when the CDN is off)."
  value       = module.stack.cdn_domain_name
}

output "events_queue_url" {
  description = "Domain events queue."
  value       = module.stack.events_queue_url
}

output "events_dlq_url" {
  description = "Dead-letter queue: inspect and redrive from the SQS console."
  value       = module.stack.events_dlq_url
}

output "secret_names" {
  description = "Secrets to fill with put-secret-value (names only, never values)."
  value       = module.stack.secret_names
}

output "secret_prefix" {
  description = "SECRET_PREFIX for admincli."
  value       = module.stack.secret_prefix
}

output "nat_public_ip" {
  description = "Lambda egress IP for partner allowlists."
  value       = module.stack.nat_public_ip
}

output "rds_address" {
  description = "Private MySQL hostname (for the SSM tunnel)."
  value       = module.stack.rds_address
}

output "redis_address" {
  description = "Private Redis host:port (for the SSM tunnel)."
  value       = module.stack.redis_address
}

output "ops_tunnel_instance_id" {
  description = "SSM target; empty when the tunnel is off."
  value       = module.stack.ops_tunnel_instance_id
}

output "function_names" {
  description = "Lambda names (migrate is invoked by apply; see infra/scripts)."
  value       = module.stack.function_names
}

output "alerts_topic_arn" {
  description = "SNS alarm topic."
  value       = module.stack.alerts_topic_arn
}
