variable "name" {
  description = "API name, e.g. athlix-dev-ws."
  type        = string
}

variable "stage_name" {
  description = "Stage name; part of the wss:// and management URLs."
  type        = string
  default     = "live"
}

variable "ws_lambda_function_name" {
  type = string
}

variable "ws_lambda_invoke_arn" {
  type = string
}

variable "manage_connections_role_names" {
  description = "IAM role names (api, ws, worker) that get execute-api:ManageConnections on this API."
  type        = list(string)
}

variable "throttling_rate_limit" {
  description = "Messages per second across the stage."
  type        = number
  default     = 100
}

variable "throttling_burst_limit" {
  type    = number
  default = 200
}

variable "enable_access_logs" {
  description = "Needs the account-level API Gateway CloudWatch Logs role (created by infra/bootstrap)."
  type        = bool
  default     = true
}

variable "log_retention_days" {
  type    = number
  default = 30
}

variable "domain_name" {
  description = "Optional custom domain for the socket, e.g. ws.example.in."
  type        = string
  default     = ""
}

variable "certificate_arn" {
  description = "ACM certificate ARN in ap-south-1 covering domain_name."
  type        = string
  default     = ""
}
