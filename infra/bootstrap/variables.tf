variable "region" {
  type    = string
  default = "ap-south-1"
}

variable "github_repo" {
  description = "owner/name of the GitHub repository allowed to run terraform plan via OIDC. Empty = no OIDC role."
  type        = string
  default     = ""

  validation {
    condition     = var.github_repo == "" || can(regex("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$", var.github_repo))
    error_message = "github_repo must look like owner/name."
  }
}

variable "create_github_oidc_provider" {
  description = "false if the account already has the token.actions.githubusercontent.com provider (only one may exist)."
  type        = bool
  default     = true
}
