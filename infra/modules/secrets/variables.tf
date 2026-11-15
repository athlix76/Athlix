variable "prefix" {
  description = "SECRET_PREFIX, e.g. athlix/dev. Full names are <prefix>/<short>."
  type        = string

  validation {
    condition     = can(regex("^athlix/(dev|prod)$", var.prefix))
    error_message = "prefix must be athlix/dev or athlix/prod (the backend builds names as SECRET_PREFIX + \"/\" + short)."
  }
}

variable "recovery_window_days" {
  description = "Days a deleted secret can be restored. 0 deletes immediately (dev only: lets you destroy and recreate with the same names)."
  type        = number
  default     = 30

  validation {
    condition     = var.recovery_window_days == 0 || (var.recovery_window_days >= 7 && var.recovery_window_days <= 30)
    error_message = "recovery_window_days must be 0 or between 7 and 30."
  }
}
