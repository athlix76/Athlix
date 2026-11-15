output "bucket_name" {
  description = "MEDIA_BUCKET."
  value       = aws_s3_bucket.media.id
}

output "bucket_arn" {
  value = aws_s3_bucket.media.arn
}

output "cdn_domain_name" {
  description = "CloudFront domain (empty when the CDN is disabled)."
  value       = var.enable_cdn ? aws_cloudfront_distribution.media[0].domain_name : ""
}

output "cdn_distribution_id" {
  value = var.enable_cdn ? aws_cloudfront_distribution.media[0].id : ""
}

output "bucket_origin" {
  description = "https:// origin of presigned URLs (regional virtual-hosted host). The admin web CSP connect-src / img-src allows exactly this origin."
  value       = "https://${aws_s3_bucket.media.bucket_regional_domain_name}"
}
