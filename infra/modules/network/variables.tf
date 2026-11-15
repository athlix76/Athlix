variable "name" {
  description = "Name prefix for every resource, e.g. athlix-dev."
  type        = string
}

variable "region" {
  description = "AWS region, used to build VPC endpoint service names."
  type        = string
}

variable "vpc_cidr" {
  description = "VPC CIDR. A /16 is split into /24 subnets."
  type        = string
  default     = "10.20.0.0/16"
}

variable "enable_interface_endpoints" {
  description = "Create the Secrets Manager and SQS interface endpoints."
  type        = bool
  default     = true
}

variable "interface_endpoint_subnet_count" {
  description = "How many private subnets (AZs) get an interface endpoint ENI: 1 (cheaper) or 2 (survives an AZ outage)."
  type        = number
  default     = 2

  validation {
    condition     = contains([1, 2], var.interface_endpoint_subnet_count)
    error_message = "interface_endpoint_subnet_count must be 1 or 2."
  }
}
