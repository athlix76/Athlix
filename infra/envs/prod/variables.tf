# Set these in terraform.tfvars (copy terraform.tfvars.example). tfvars files are gitignored
# and must never contain a secret: secret values go into Secrets Manager only.

variable "region" {
  type    = string
  default = "ap-south-1"
}

variable "alert_emails" {
  description = "Who gets alarms and budget alerts."
  type        = list(string)
}

variable "monthly_budget_usd" {
  description = "Monthly AWS Budgets limit for this environment (0 = none)."
  type        = number
  default     = 450
}

variable "admin_origins" {
  description = "Admin web origins, e.g. [\"https://admin.example.com\"]."
  type        = list(string)
  default     = []
}

variable "app_deep_link_base" {
  type    = string
  default = ""
}

variable "certificate_arn" {
  description = "Regional ACM certificate in ap-south-1. Empty = no custom domains."
  type        = string
  default     = ""
}

variable "api_domain_name" {
  type    = string
  default = ""
}

variable "ws_domain_name" {
  type    = string
  default = ""
}

variable "enable_cdn" {
  description = "CloudFront in front of public media prefixes (event banners)."
  type        = bool
  default     = false
}

variable "api_reserved_concurrency" {
  description = "Needs the Lambda concurrency quota raised to 1,000 first; -1 until then."
  type        = number
  default     = 100
}

variable "ws_reserved_concurrency" {
  type    = number
  default = 50
}

variable "worker_reserved_concurrency" {
  type    = number
  default = 10
}

variable "disabled_jobs" {
  type    = list(string)
  default = []
}

variable "enable_ops_tunnel" {
  description = "Create the SSM port-forwarding instance only while it is needed."
  type        = bool
  default     = false
}
