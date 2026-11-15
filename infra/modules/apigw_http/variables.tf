variable "name" {
  description = "API name, e.g. athlix-dev-http."
  type        = string
}

variable "lambda_function_name" {
  description = "Name of the api Lambda (for the invoke permission)."
  type        = string
}

variable "lambda_invoke_arn" {
  description = "invoke_arn of the api Lambda."
  type        = string
}

variable "throttling_rate_limit" {
  description = "Steady-state requests per second across the whole API."
  type        = number
  default     = 100
}

variable "throttling_burst_limit" {
  description = "Burst bucket size (requests)."
  type        = number
  default     = 200
}

variable "log_retention_days" {
  type    = number
  default = 30
}

variable "domain_name" {
  description = "Optional custom domain, e.g. api.example.in. Empty = use the execute-api URL."
  type        = string
  default     = ""
}

variable "certificate_arn" {
  description = "ACM certificate ARN in the SAME region (ap-south-1) covering domain_name. Required when domain_name is set."
  type        = string
  default     = ""

  validation {
    condition     = var.certificate_arn == "" || can(regex("^arn:aws:acm:", var.certificate_arn))
    error_message = "certificate_arn must be an ACM certificate ARN."
  }
}
