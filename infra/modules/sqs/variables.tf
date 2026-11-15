variable "name" {
  description = "Queue name, athlix-<env>-events. The DLQ is <name>-dlq."
  type        = string
}

variable "worker_function_arn" {
  type = string
}

variable "visibility_timeout_seconds" {
  description = "AWS guidance: at least 6x the worker timeout (120 s) so in-flight batches are not redelivered."
  type        = number
  default     = 720
}

variable "message_retention_seconds" {
  type    = number
  default = 345600 # 4 days
}

variable "max_receive_count" {
  type    = number
  default = 5
}

variable "batch_size" {
  type    = number
  default = 10
}

variable "maximum_batching_window_seconds" {
  description = "Wait up to this long to fill a batch. 0 = lowest latency for push notifications."
  type        = number
  default     = 0
}

variable "maximum_concurrency" {
  description = "Max concurrent worker invocations from SQS (2 to 1000); protects RDS connections."
  type        = number
  default     = 2

  validation {
    condition     = var.maximum_concurrency >= 2 && var.maximum_concurrency <= 1000
    error_message = "maximum_concurrency must be between 2 and 1000."
  }
}
