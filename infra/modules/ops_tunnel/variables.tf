variable "name" {
  description = "Name prefix, e.g. athlix-dev."
  type        = string
}

variable "subnet_id" {
  description = "A private subnet (egress via NAT for Session Manager)."
  type        = string
}

variable "security_group_id" {
  description = "The network module's ops security group (no ingress; allowed into RDS and Redis)."
  type        = string
}

variable "instance_type" {
  type    = string
  default = "t4g.nano"
}
