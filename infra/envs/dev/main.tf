# Athlix dev: the smallest sizes that still run the real architecture (VPC, RDS, Redis,
# Lambdas, both API Gateways, SQS, Scheduler). Everything is sized for a handful of testers.

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project     = "athlix"
      Environment = "dev"
      ManagedBy   = "terraform"
    }
  }
}

module "stack" {
  source = "../../modules/stack"

  environment     = "dev"
  region          = var.region
  lambda_dist_dir = "${path.module}/../../../backend/dist"
  log_level       = "debug"

  app_deep_link_base = var.app_deep_link_base
  admin_origins      = var.admin_origins
  certificate_arn    = var.certificate_arn
  api_domain_name    = var.api_domain_name
  ws_domain_name     = var.ws_domain_name

  # One interface-endpoint ENI each (about $8/month per endpoint per AZ).
  interface_endpoint_subnet_count = 1
  log_retention_days              = 14

  # Secrets are deleted immediately on destroy so a rebuilt dev can reuse the names.
  secrets_recovery_window_days = 0

  # Smallest burstable Graviton sizes; single AZ; 1 day of backups; easy teardown.
  db_instance_class           = "db.t4g.micro"
  db_allocated_storage_gb     = 20
  db_max_allocated_storage_gb = 50
  db_multi_az                 = false
  db_backup_retention_days    = 1
  db_deletion_protection      = false
  db_final_snapshot           = false
  db_performance_insights     = false

  redis_node_type          = "cache.t4g.micro"
  redis_num_cache_clusters = 1

  media_force_destroy = true
  media_versioning    = false
  enable_cdn          = false

  # Unreserved: a new account's concurrency quota is often only 10, and reserving fails there.
  api_reserved_concurrency    = -1
  ws_reserved_concurrency     = -1
  worker_reserved_concurrency = -1
  worker_sqs_max_concurrency  = 2

  http_throttling_rate_limit  = 50
  http_throttling_burst_limit = 100
  ws_throttling_rate_limit    = 50
  ws_throttling_burst_limit   = 100
  ws_access_logs              = var.ws_access_logs

  disabled_jobs = var.disabled_jobs

  alert_emails       = var.alert_emails
  monthly_budget_usd = var.monthly_budget_usd

  enable_ops_tunnel = var.enable_ops_tunnel
}
