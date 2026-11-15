variable "name" {
  description = "Replication group id and name prefix, e.g. athlix-dev (max 40 characters)."
  type        = string
}

variable "vpc_id" {
  type = string
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "allowed_security_group_ids" {
  description = "Security groups allowed to reach Redis on 6379."
  type        = list(string)
}

variable "engine_version" {
  type    = string
  default = "7.1"
}

variable "node_type" {
  type    = string
  default = "cache.t4g.micro"
}

variable "num_cache_clusters" {
  description = "1 = single node (dev). 2 = primary + replica in another AZ with automatic failover (prod)."
  type        = number
  default     = 1

  validation {
    condition     = var.num_cache_clusters >= 1 && var.num_cache_clusters <= 6
    error_message = "num_cache_clusters must be between 1 and 6."
  }
}

variable "transit_encryption_mode" {
  description = "required (TLS only; the backend dials TLS outside local). preferred allows plain text too."
  type        = string
  default     = "required"

  validation {
    condition     = contains(["preferred", "required"], var.transit_encryption_mode)
    error_message = "transit_encryption_mode must be preferred or required."
  }
}

variable "snapshot_retention_days" {
  description = "Daily snapshots kept. 0 disables them (fine: Redis holds only short-lived state)."
  type        = number
  default     = 0
}

variable "apply_immediately" {
  type    = bool
  default = false
}
