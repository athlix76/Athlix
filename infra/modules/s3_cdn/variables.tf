variable "bucket_name" {
  description = "Globally unique bucket name, e.g. athlix-dev-media-<account id>."
  type        = string
}

variable "force_destroy" {
  description = "Allow terraform destroy to delete a non-empty bucket (dev only)."
  type        = bool
  default     = false
}

variable "versioning" {
  description = "Keep previous versions of overwritten / deleted objects (recovery from mistakes)."
  type        = bool
  default     = true
}

variable "noncurrent_version_days" {
  description = "Days a previous object version is kept before it is deleted."
  type        = number
  default     = 30
}

variable "cors_allowed_origins" {
  description = "Browser origins allowed to PUT/GET presigned URLs (the admin panel). Never \"*\"."
  type        = list(string)
  default     = []

  validation {
    condition     = !contains(var.cors_allowed_origins, "*")
    error_message = "Do not allow every origin."
  }
}

variable "enable_cdn" {
  description = "Create a CloudFront distribution (OAC) for the public-safe prefixes. The backend currently serves media through presigned S3 URLs, so this is off by default."
  type        = bool
  default     = false
}

variable "cdn_public_prefixes" {
  description = "Key prefixes CloudFront may read, from the purposes in platform/media. Only content that is public anyway."
  type        = list(string)
  default     = ["event_banner"]

  validation {
    condition = alltrue([
      for p in var.cdn_public_prefixes : !contains(["organiser_document", "achievement_certificate", "chat_image", "profile_photo", "location_proof"], trimsuffix(p, "/"))
    ])
    error_message = "organiser_document, achievement_certificate, chat_image, profile_photo and location_proof must never be served without an access check."
  }
}

variable "cdn_price_class" {
  description = "PriceClass_200 includes India edge locations at lower cost than PriceClass_All."
  type        = string
  default     = "PriceClass_200"
}
