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
  default     = 120
}

variable "admin_origins" {
  description = "Admin web origins, e.g. [\"http://localhost:5050\"]."
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

variable "ws_access_logs" {
  description = "Set false if infra/bootstrap (API Gateway CloudWatch role) has not been applied in this account."
  type        = bool
  default     = true
}

variable "disabled_jobs" {
  type    = list(string)
  default = []
}

variable "enable_ops_tunnel" {
  description = "Create the SSM port-forwarding instance (admincli, one-off queries)."
  type        = bool
  default     = false
}
