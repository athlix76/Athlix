variable "name" {
  description = "Name prefix, e.g. athlix-dev."
  type        = string
}

variable "environment" {
  description = "dev or prod (value of the Environment tag, for the optional budget filter)."
  type        = string
}

variable "alert_emails" {
  description = "Addresses that get alarm and budget emails."
  type        = list(string)

  validation {
    condition     = length(var.alert_emails) > 0
    error_message = "Give at least one alert email."
  }
}

variable "lambda_function_names" {
  type = list(string)
}

variable "http_api_id" {
  type = string
}

variable "rds_instance_id" {
  type = string
}

variable "redis_node_ids" {
  type    = list(string)
  default = []
}

variable "queue_name" {
  type = string
}

variable "dlq_name" {
  type = string
}

variable "lambda_error_threshold" {
  description = "Errors per 5 minutes (per function) that raise an alarm."
  type        = number
  default     = 5
}

variable "http_5xx_threshold" {
  description = "5xx responses per 5 minutes that raise an alarm."
  type        = number
  default     = 10
}

variable "rds_cpu_threshold" {
  type    = number
  default = 80
}

variable "rds_free_storage_gb" {
  type    = number
  default = 2
}

variable "monthly_budget_usd" {
  description = "Monthly cost budget in USD. 0 = no budget from Terraform (you still create the account-wide one by hand first)."
  type        = number
  default     = 0
}

variable "budget_env_tag_filter" {
  description = "Limit the budget to resources tagged Environment=<environment> (needs the cost allocation tag activated)."
  type        = bool
  default     = false
}
