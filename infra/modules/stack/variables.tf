# ---------------------------------------------------------------- identity

variable "environment" {
  description = "dev or prod. Drives every name (athlix-<env>-*) and the secret prefix (athlix/<env>)."
  type        = string

  validation {
    condition     = contains(["dev", "prod"], var.environment)
    error_message = "environment must be dev or prod."
  }
}

variable "region" {
  description = "AWS region (also set on the provider by the env root)."
  type        = string
  default     = "ap-south-1"
}

variable "lambda_dist_dir" {
  description = "Folder holding api.zip, ws.zip, worker.zip and migrate.zip from `make -C backend build-lambda`."
  type        = string
}

variable "log_level" {
  type    = string
  default = "info"

  validation {
    condition     = contains(["debug", "info", "warn", "error"], var.log_level)
    error_message = "log_level must be debug, info, warn or error."
  }
}

variable "log_retention_days" {
  description = "CloudWatch retention for Lambda and API Gateway logs."
  type        = number
  default     = 14
}

# ---------------------------------------------------------------- app-facing values (not secret)

variable "app_deep_link_base" {
  description = "Base URL for deep links in notifications and emails, e.g. https://athlix.app."
  type        = string
  default     = ""
}

variable "admin_origins" {
  description = "Origins of the admin web app (CORS on the API and on presigned PUTs). Never \"*\"."
  type        = list(string)
  default     = []
}

variable "certificate_arn" {
  description = "Regional ACM certificate (ap-south-1) covering api_domain_name and ws_domain_name. Empty = no custom domains."
  type        = string
  default     = ""
}

variable "api_domain_name" {
  description = "Custom domain for the HTTP API, e.g. api.dev.example.com. Empty = use the execute-api URL."
  type        = string
  default     = ""
}

variable "ws_domain_name" {
  description = "Custom domain for the WebSocket API, e.g. ws.dev.example.com. Empty = use the execute-api URL."
  type        = string
  default     = ""
}

# ---------------------------------------------------------------- network

variable "vpc_cidr" {
  type    = string
  default = "10.20.0.0/16"
}

variable "enable_interface_endpoints" {
  description = "Secrets Manager and SQS interface endpoints (keeps that traffic off the NAT)."
  type        = bool
  default     = true
}

variable "interface_endpoint_subnet_count" {
  description = "1 (cheaper, dev) or 2 (survives an AZ outage, prod)."
  type        = number
  default     = 1
}

# ---------------------------------------------------------------- secrets

variable "secrets_recovery_window_days" {
  description = "0 deletes a secret immediately on destroy (dev); 7-30 keeps it recoverable (prod)."
  type        = number
  default     = 0
}

variable "function_secrets" {
  description = "Secret short names each function may read. Each function's role gets GetSecretValue on exactly these ARNs."
  type        = map(list(string))
  default = {
    api     = ["db", "jwt", "pii-keys", "razorpay", "digilocker", "kyc-aggregator", "msg91", "fcm", "google-play", "app-store"]
    ws      = ["db", "jwt", "pii-keys"]
    worker  = ["db", "jwt", "pii-keys", "msg91", "fcm", "razorpay", "google-play", "app-store"]
    migrate = ["db"]
  }

  validation {
    condition     = alltrue([for k in ["api", "ws", "worker", "migrate"] : contains(keys(var.function_secrets), k)])
    error_message = "function_secrets must have entries for api, ws, worker and migrate."
  }
}

# ---------------------------------------------------------------- database

variable "db_instance_class" {
  type    = string
  default = "db.t4g.micro"
}

variable "db_allocated_storage_gb" {
  type    = number
  default = 20
}

variable "db_max_allocated_storage_gb" {
  description = "Storage autoscaling ceiling."
  type        = number
  default     = 100
}

variable "db_multi_az" {
  type    = bool
  default = false
}

variable "db_backup_retention_days" {
  type    = number
  default = 1
}

variable "db_deletion_protection" {
  type    = bool
  default = false
}

variable "db_final_snapshot" {
  description = "Take a final snapshot when the instance is destroyed."
  type        = bool
  default     = false
}

variable "db_performance_insights" {
  description = "Performance Insights (not supported on db.t4g.micro / small)."
  type        = bool
  default     = false
}

variable "run_migrations_on_apply" {
  description = "Invoke the migrate Lambda during apply whenever its code changes (before api / ws / worker are updated)."
  type        = bool
  default     = true
}

# ---------------------------------------------------------------- redis

variable "redis_node_type" {
  type    = string
  default = "cache.t4g.micro"
}

variable "redis_num_cache_clusters" {
  description = "1 = single node; 2+ = primary + replicas with automatic failover."
  type        = number
  default     = 1
}

variable "redis_transit_encryption_mode" {
  description = "required (the backend dials TLS outside local); preferred also allows plain text."
  type        = string
  default     = "required"
}

# ---------------------------------------------------------------- media

variable "media_force_destroy" {
  description = "Let terraform destroy delete a non-empty media bucket (dev only)."
  type        = bool
  default     = false
}

variable "media_versioning" {
  type    = bool
  default = true
}

variable "enable_cdn" {
  type    = bool
  default = false
}

variable "cdn_public_prefixes" {
  description = "Media key prefixes CloudFront may serve. Only purposes that are public in the app."
  type        = list(string)
  default     = ["event_banner"]
}

# ---------------------------------------------------------------- capacity

variable "api_reserved_concurrency" {
  description = "-1 = unreserved (new accounts have a small concurrency quota; reserving fails there)."
  type        = number
  default     = -1
}

variable "ws_reserved_concurrency" {
  type    = number
  default = -1
}

variable "worker_reserved_concurrency" {
  type    = number
  default = -1
}

variable "worker_sqs_max_concurrency" {
  description = "Maximum worker instances the SQS event source mapping may run at once (minimum 2)."
  type        = number
  default     = 2
}

variable "http_throttling_rate_limit" {
  description = "Steady requests per second across the HTTP API stage."
  type        = number
  default     = 50
}

variable "http_throttling_burst_limit" {
  type    = number
  default = 100
}

variable "ws_throttling_rate_limit" {
  type    = number
  default = 50
}

variable "ws_throttling_burst_limit" {
  type    = number
  default = 100
}

variable "ws_access_logs" {
  description = "WebSocket stage access logs. Needs the account-level API Gateway CloudWatch role (infra/bootstrap)."
  type        = bool
  default     = true
}

variable "disabled_jobs" {
  description = "Scheduled job names to leave out (e.g. while a job's code is not deployed yet)."
  type        = list(string)
  default     = []
}

# ---------------------------------------------------------------- monitoring and cost

variable "alert_emails" {
  description = "Addresses subscribed to alarms and budget alerts (each must confirm the SNS email)."
  type        = list(string)
}

variable "monthly_budget_usd" {
  description = "AWS Budgets monthly limit in USD. 0 = no budget from this stack."
  type        = number
  default     = 0
}

variable "budget_env_tag_filter" {
  description = "Filter the budget on Environment=<env>. Needs the Environment cost allocation tag activated in Billing."
  type        = bool
  default     = false
}

# ---------------------------------------------------------------- operator access

variable "enable_ops_tunnel" {
  description = "Create the t4g.nano SSM port-forwarding instance (for admincli / one-off queries). Turn off when done."
  type        = bool
  default     = false
}
