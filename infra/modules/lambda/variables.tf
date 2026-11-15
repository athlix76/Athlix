variable "function_name" {
  description = "Function name, e.g. athlix-dev-api. Also the IAM role name and the log group suffix."
  type        = string
}

variable "description" {
  type    = string
  default = ""
}

variable "zip_path" {
  description = "Path to the zip from `make build-lambda` (backend/dist/<name>.zip, a single bootstrap binary)."
  type        = string
}

variable "memory_mb" {
  type    = number
  default = 256
}

variable "timeout_seconds" {
  type    = number
  default = 29
}

variable "reserved_concurrency" {
  description = "Reserved concurrency (caps DB connections). -1 = unreserved. New accounts have a low concurrency quota: request an increase before reserving."
  type        = number
  default     = -1
}

variable "subnet_ids" {
  description = "Private subnets (RDS and Redis are only reachable from inside the VPC)."
  type        = list(string)
}

variable "security_group_ids" {
  type = list(string)
}

variable "log_retention_days" {
  type    = number
  default = 30
}

variable "secret_arns" {
  description = "Exact ARNs of the Secrets Manager secrets this function may read."
  type        = list(string)
  default     = []
}

variable "policy_statements" {
  description = "Extra Allow statements (S3, SQS, execute-api ...). Keep resources exact; never \"*\"."
  type = list(object({
    sid       = string
    actions   = list(string)
    resources = list(string)
  }))
  default = []

  validation {
    condition     = alltrue([for st in var.policy_statements : !contains(st.resources, "*")])
    error_message = "policy_statements must name exact resources, not \"*\"."
  }
}

variable "environment" {
  description = "Environment variables. Only non-secret values and secret NAMES (SECRET_PREFIX); never a secret value."
  type        = map(string)
  default     = {}

  # Allowlist of the bootstrap variables backend/internal/platform/config reads. A new key
  # must be added here on purpose, which is the moment to check it is not a secret value.
  validation {
    condition = alltrue([
      for k in keys(var.environment) : contains([
        "ENV",
        "SECRET_PREFIX",
        "REDIS_ADDR",
        "MEDIA_BUCKET",
        "EVENTS_QUEUE_URL",
        "WS_MANAGEMENT_URL",
        "WS_PUBLIC_URL",
        "PUBLIC_API_BASE_URL",
        "APP_DEEP_LINK_BASE",
        "ADMIN_ORIGINS",
        "LOG_LEVEL",
      ], k)
    ])
    error_message = "Unknown environment variable. Only non-secret config keys are allowed (DB_DSN, DEV_FIXED_OTP and secret values are never set in AWS)."
  }
}
