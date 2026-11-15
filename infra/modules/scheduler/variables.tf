variable "name" {
  description = "Schedule group name and role prefix, e.g. athlix-dev."
  type        = string
}

variable "account_id" {
  description = "AWS account id (confused-deputy guard on the scheduler role)."
  type        = string
}

variable "worker_function_arn" {
  type = string
}

variable "disabled_jobs" {
  description = "Job names to leave unscheduled (e.g. while debugging). Run them by hand with aws lambda invoke."
  type        = list(string)
  default     = []
}
