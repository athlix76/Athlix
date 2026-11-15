# The whole Athlix environment. infra/envs/dev and infra/envs/prod call this module with
# different sizes; nothing else differs between environments.

data "aws_caller_identity" "current" {}

locals {
  name          = "athlix-${var.environment}"
  secret_prefix = "athlix/${var.environment}"
  account_id    = data.aws_caller_identity.current.account_id

  fn = {
    api     = "${local.name}-api"
    ws      = "${local.name}-ws"
    worker  = "${local.name}-worker"
    migrate = "${local.name}-migrate"
  }

  zip = { for k, _ in local.fn : k => "${var.lambda_dist_dir}/${k}.zip" }

  secret_arns = {
    for k, shorts in var.function_secrets : k => [for s in shorts : module.secrets.arns[s]]
  }

  # Environment variables: only non-secret values and the secret PREFIX (never a value).
  # config.Load() (api, ws, worker) requires REDIS_ADDR and MEDIA_BUCKET outside ENV=local;
  # migrate uses config.LoadDBOnly() and ignores them.
  base_env = {
    ENV           = var.environment
    SECRET_PREFIX = local.secret_prefix
    REDIS_ADDR    = module.redis.address
    MEDIA_BUCKET  = module.media.bucket_name
    LOG_LEVEL     = var.log_level
  }

  app_env = merge(local.base_env, {
    EVENTS_QUEUE_URL    = module.sqs.queue_url
    WS_MANAGEMENT_URL   = module.ws_api.management_url
    WS_PUBLIC_URL       = module.ws_api.public_url
    PUBLIC_API_BASE_URL = module.http_api.base_url
    APP_DEEP_LINK_BASE  = var.app_deep_link_base
    ADMIN_ORIGINS       = join(",", var.admin_origins)
  })

  # IAM statements shared by the functions (exact ARNs only).
  st_media_rw = {
    sid       = "MediaObjects"
    actions   = ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"]
    resources = ["${module.media.bucket_arn}/*"]
  }
  st_media_read = {
    sid       = "MediaRead"
    actions   = ["s3:GetObject"]
    resources = ["${module.media.bucket_arn}/*"]
  }
  # ListBucket lets HeadObject return 404 (not 403) for a missing upload.
  st_media_list = {
    sid       = "MediaList"
    actions   = ["s3:ListBucket"]
    resources = [module.media.bucket_arn]
  }
  st_events_send = {
    sid       = "PublishEvents"
    actions   = ["sqs:SendMessage"]
    resources = [module.sqs.queue_arn]
  }
  st_events_consume = {
    sid       = "ConsumeEvents"
    actions   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes", "sqs:ChangeMessageVisibility"]
    resources = [module.sqs.queue_arn]
  }
}

# ---------------------------------------------------------------- foundation

module "network" {
  source = "../network"

  name                            = local.name
  region                          = var.region
  vpc_cidr                        = var.vpc_cidr
  enable_interface_endpoints      = var.enable_interface_endpoints
  interface_endpoint_subnet_count = var.interface_endpoint_subnet_count
}

module "secrets" {
  source = "../secrets"

  prefix               = local.secret_prefix
  recovery_window_days = var.secrets_recovery_window_days
}

module "rds" {
  source = "../rds"

  name                         = local.name
  vpc_id                       = module.network.vpc_id
  private_subnet_ids           = module.network.private_subnet_ids
  allowed_security_group_ids   = [module.network.lambda_security_group_id, module.network.ops_security_group_id]
  db_secret_id                 = module.secrets.arns["db"]
  instance_class               = var.db_instance_class
  allocated_storage_gb         = var.db_allocated_storage_gb
  max_allocated_storage_gb     = var.db_max_allocated_storage_gb
  multi_az                     = var.db_multi_az
  backup_retention_days        = var.db_backup_retention_days
  deletion_protection          = var.db_deletion_protection
  final_snapshot               = var.db_final_snapshot
  performance_insights_enabled = var.db_performance_insights
}

module "redis" {
  source = "../redis"

  name                       = local.name
  vpc_id                     = module.network.vpc_id
  private_subnet_ids         = module.network.private_subnet_ids
  allowed_security_group_ids = [module.network.lambda_security_group_id, module.network.ops_security_group_id]
  node_type                  = var.redis_node_type
  num_cache_clusters         = var.redis_num_cache_clusters
  transit_encryption_mode    = var.redis_transit_encryption_mode
}

module "media" {
  source = "../s3_cdn"

  bucket_name          = "${local.name}-media-${local.account_id}"
  force_destroy        = var.media_force_destroy
  versioning           = var.media_versioning
  cors_allowed_origins = var.admin_origins
  enable_cdn           = var.enable_cdn
  cdn_public_prefixes  = var.cdn_public_prefixes
}

# ---------------------------------------------------------------- migrate (runs first)

module "migrate" {
  source = "../lambda"

  function_name      = local.fn.migrate
  description        = "Applies backend/migrations (invoked once per deploy)"
  zip_path           = local.zip.migrate
  memory_mb          = 256
  timeout_seconds    = 300
  subnet_ids         = module.network.private_subnet_ids
  security_group_ids = [module.network.lambda_security_group_id]
  log_retention_days = var.log_retention_days
  secret_arns        = local.secret_arns["migrate"]
  environment        = local.base_env
}

# Runs the migrations during `terraform apply`, before the api / ws / worker code is updated
# (those modules depend on this). Re-runs whenever the migrate zip changes, i.e. on every
# deploy that rebuilt it. A failed migration fails the apply and leaves the old app code
# running. Migrations are forward-only and idempotent (schema_migrations).
resource "aws_lambda_invocation" "migrate" {
  count = var.run_migrations_on_apply ? 1 : 0

  function_name = module.migrate.function_name
  input         = jsonencode({ trigger = "terraform-apply" })

  triggers = {
    code      = module.migrate.source_code_hash
    db_secret = module.rds.secret_version_id
  }

  depends_on = [module.rds, module.redis, module.network]
}

# ---------------------------------------------------------------- application Lambdas

module "api" {
  source = "../lambda"

  function_name        = local.fn.api
  description          = "Athlix REST API (cmd/api) behind the HTTP API"
  zip_path             = local.zip.api
  memory_mb            = 512
  timeout_seconds      = 29
  reserved_concurrency = var.api_reserved_concurrency
  subnet_ids           = module.network.private_subnet_ids
  security_group_ids   = [module.network.lambda_security_group_id]
  log_retention_days   = var.log_retention_days
  secret_arns          = local.secret_arns["api"]
  environment          = local.app_env
  policy_statements    = [local.st_media_rw, local.st_media_list, local.st_events_send]

  depends_on = [aws_lambda_invocation.migrate]
}

module "ws" {
  source = "../lambda"

  function_name        = local.fn.ws
  description          = "Athlix chat WebSocket handler (cmd/ws)"
  zip_path             = local.zip.ws
  memory_mb            = 256
  timeout_seconds      = 29
  reserved_concurrency = var.ws_reserved_concurrency
  subnet_ids           = module.network.private_subnet_ids
  security_group_ids   = [module.network.lambda_security_group_id]
  log_retention_days   = var.log_retention_days
  secret_arns          = local.secret_arns["ws"]
  environment          = local.app_env
  policy_statements    = [local.st_media_read, local.st_events_send]

  depends_on = [aws_lambda_invocation.migrate]
}

module "worker" {
  source = "../lambda"

  function_name        = local.fn.worker
  description          = "Athlix domain events (SQS) and scheduled jobs (cmd/worker)"
  zip_path             = local.zip.worker
  memory_mb            = 512
  timeout_seconds      = 120
  reserved_concurrency = var.worker_reserved_concurrency
  subnet_ids           = module.network.private_subnet_ids
  security_group_ids   = [module.network.lambda_security_group_id]
  log_retention_days   = var.log_retention_days
  secret_arns          = local.secret_arns["worker"]
  environment          = local.app_env
  policy_statements    = [local.st_media_rw, local.st_media_list, local.st_events_send, local.st_events_consume]

  depends_on = [aws_lambda_invocation.migrate]
}

# ---------------------------------------------------------------- entry points and triggers

module "http_api" {
  source = "../apigw_http"

  name                   = "${local.name}-http"
  lambda_function_name   = module.api.function_name
  lambda_invoke_arn      = module.api.invoke_arn
  throttling_rate_limit  = var.http_throttling_rate_limit
  throttling_burst_limit = var.http_throttling_burst_limit
  log_retention_days     = var.log_retention_days
  domain_name            = var.api_domain_name
  certificate_arn        = var.certificate_arn
}

module "ws_api" {
  source = "../apigw_ws"

  name                          = "${local.name}-ws"
  ws_lambda_function_name       = module.ws.function_name
  ws_lambda_invoke_arn          = module.ws.invoke_arn
  manage_connections_role_names = [module.api.role_name, module.ws.role_name, module.worker.role_name]
  throttling_rate_limit         = var.ws_throttling_rate_limit
  throttling_burst_limit        = var.ws_throttling_burst_limit
  enable_access_logs            = var.ws_access_logs
  log_retention_days            = var.log_retention_days
  domain_name                   = var.ws_domain_name
  certificate_arn               = var.certificate_arn
}

module "sqs" {
  source = "../sqs"

  name                = "${local.name}-events"
  worker_function_arn = module.worker.arn
  maximum_concurrency = var.worker_sqs_max_concurrency
}

module "scheduler" {
  source = "../scheduler"

  name                = local.name
  account_id          = local.account_id
  worker_function_arn = module.worker.arn
  disabled_jobs       = var.disabled_jobs
}

module "monitoring" {
  source = "../monitoring"

  name                  = local.name
  environment           = var.environment
  alert_emails          = var.alert_emails
  lambda_function_names = values(local.fn) # plain names so the alarm for_each is known at plan
  http_api_id           = module.http_api.api_id
  rds_instance_id       = module.rds.instance_id
  redis_node_ids        = module.redis.node_ids
  queue_name            = module.sqs.queue_name
  dlq_name              = module.sqs.dlq_name
  monthly_budget_usd    = var.monthly_budget_usd
  budget_env_tag_filter = var.budget_env_tag_filter
}

# ---------------------------------------------------------------- optional operator access

module "ops_tunnel" {
  source = "../ops_tunnel"
  count  = var.enable_ops_tunnel ? 1 : 0

  name              = local.name
  subnet_id         = module.network.private_subnet_ids[0]
  security_group_id = module.network.ops_security_group_id
}
