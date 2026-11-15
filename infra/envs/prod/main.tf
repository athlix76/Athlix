# Athlix prod, sized for 10,000 to 1,00,000 (1 lakh) registered users.
#
# Working assumption behind the numbers: about 10% of users are active on a given day and
# peak traffic is concentrated in the evening, so at 1 lakh users expect roughly 10k daily
# actives and a peak of 100-300 API requests per second. Lambda and API Gateway scale with
# that on their own; the parts that need explicit sizing are the database (connections and
# CPU), Redis memory, and the concurrency caps that protect the database.

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project     = "athlix"
      Environment = "prod"
      ManagedBy   = "terraform"
    }
  }
}

module "stack" {
  source = "../../modules/stack"

  environment     = "prod"
  region          = var.region
  lambda_dist_dir = "${path.module}/../../../backend/dist"
  log_level       = "info"

  app_deep_link_base = var.app_deep_link_base
  admin_origins      = var.admin_origins
  certificate_arn    = var.certificate_arn
  api_domain_name    = var.api_domain_name
  ws_domain_name     = var.ws_domain_name

  # Endpoint ENIs in both AZs so Secrets Manager / SQS keep working if one AZ fails.
  # There is still a single NAT gateway (see modules/network): an AZ outage in the NAT's
  # AZ stops outbound partner calls (Razorpay, MSG91, FCM ...) until it recovers, but not
  # the database, Redis, S3, SQS or Secrets Manager. A second NAT costs about $41/month.
  interface_endpoint_subnet_count = 2
  log_retention_days              = 30

  # A deleted secret can be restored for 30 days.
  secrets_recovery_window_days = 30

  # db.t4g.medium (2 vCPU, 4 GB): comfortably handles the peak above for an OLTP workload
  # of indexed lookups, and allows a few hundred connections, which covers the Lambda
  # concurrency caps below times the backend's per-instance pool. Multi-AZ gives a
  # standby in the second AZ with automatic failover (1-2 minutes). Step up to
  # db.t4g.large or db.r7g.large when the CPU alarm (80%) fires regularly; consider RDS
  # Proxy if connection count, not CPU, becomes the limit.
  db_instance_class           = "db.t4g.medium"
  db_allocated_storage_gb     = 50
  db_max_allocated_storage_gb = 500
  db_multi_az                 = true
  db_backup_retention_days    = 14
  db_deletion_protection      = true
  db_final_snapshot           = true
  db_performance_insights     = true # 7-day retention is free; not available on micro/small

  # Redis holds short-lived data (OTP hashes, rate limits, WS tickets and connections,
  # caches). cache.t4g.small (1.37 GB) leaves room for 1 lakh users with WS connections,
  # as in docs/business/costs.md; two nodes in two AZs give automatic failover. At the
  # 10k-user stage cache.t4g.micro is enough and saves about $25/month.
  redis_node_type          = "cache.t4g.small"
  redis_num_cache_clusters = 2

  media_force_destroy = false
  media_versioning    = true # overwritten or deleted media stays recoverable for 30 days
  enable_cdn          = var.enable_cdn

  # Reserved concurrency caps how many Lambda instances (and so DB connection pools) can
  # exist at once, so a traffic spike queues at API Gateway instead of exhausting MySQL.
  # 100 api instances at ~50 ms per request serve about 2,000 requests per second, far
  # above the expected peak. Reserving needs the account's Lambda concurrency quota raised
  # to 1,000 (Service Quotas, usually granted in a day); until then set these to -1.
  api_reserved_concurrency    = var.api_reserved_concurrency
  ws_reserved_concurrency     = var.ws_reserved_concurrency
  worker_reserved_concurrency = var.worker_reserved_concurrency
  worker_sqs_max_concurrency  = 5 # leaves worker headroom for scheduled jobs

  # Stage-wide API Gateway limits: about 2-5x the expected peak, low enough to blunt a flood.
  http_throttling_rate_limit  = 500
  http_throttling_burst_limit = 1000
  ws_throttling_rate_limit    = 200
  ws_throttling_burst_limit   = 400
  ws_access_logs              = true

  disabled_jobs = var.disabled_jobs

  alert_emails       = var.alert_emails
  monthly_budget_usd = var.monthly_budget_usd

  enable_ops_tunnel = var.enable_ops_tunnel
}
