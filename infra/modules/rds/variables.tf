variable "name" {
  description = "Instance identifier and name prefix, e.g. athlix-dev."
  type        = string
}

variable "vpc_id" {
  type = string
}

variable "private_subnet_ids" {
  description = "Private subnets in two AZs for the DB subnet group."
  type        = list(string)
}

variable "allowed_security_group_ids" {
  description = "Security groups allowed to reach MySQL on 3306 (the Lambda SG, plus the ops tunnel SG)."
  type        = list(string)
}

variable "db_secret_id" {
  description = "ARN of the empty <prefix>/db secret container from modules/secrets; this module writes its value."
  type        = string
}

variable "engine_version" {
  description = "MySQL version. A major.minor like 8.0 lets RDS pick the default patch and apply minor upgrades."
  type        = string
  default     = "8.0"
}

variable "instance_class" {
  type    = string
  default = "db.t4g.micro"
}

variable "allocated_storage_gb" {
  type    = number
  default = 20
}

variable "max_allocated_storage_gb" {
  description = "Storage autoscaling ceiling. Set equal to allocated_storage_gb to disable autoscaling."
  type        = number
  default     = 100
}

variable "db_name" {
  type    = string
  default = "athlix"
}

variable "master_username" {
  type    = string
  default = "athlix_admin"
}

variable "multi_az" {
  type    = bool
  default = false
}

variable "backup_retention_days" {
  description = "Automated backup (point-in-time restore) window, 1 to 35 days."
  type        = number
  default     = 7

  validation {
    condition     = var.backup_retention_days >= 1 && var.backup_retention_days <= 35
    error_message = "backup_retention_days must be between 1 and 35 (0 would disable backups)."
  }
}

variable "deletion_protection" {
  type    = bool
  default = true
}

variable "final_snapshot" {
  description = "Take a final snapshot on destroy (keep true in prod)."
  type        = bool
  default     = true
}

variable "apply_immediately" {
  description = "Apply modifications now instead of in the maintenance window."
  type        = bool
  default     = false
}

variable "performance_insights_enabled" {
  description = "Performance Insights (7-day free retention). Not supported on db.t4g.micro / db.t4g.small for MySQL."
  type        = bool
  default     = false
}

variable "cloudwatch_logs_exports" {
  description = "RDS logs shipped to CloudWatch: any of error, slowquery, general (never general in prod: it logs bind values)."
  type        = list(string)
  default     = ["error", "slowquery"]

  validation {
    condition     = !contains(var.cloudwatch_logs_exports, "general")
    error_message = "Do not export the general log: it contains query bind values (PII)."
  }
}
