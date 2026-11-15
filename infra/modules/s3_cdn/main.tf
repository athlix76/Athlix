resource "aws_s3_bucket" "media" {
  bucket        = var.bucket_name
  force_destroy = var.force_destroy
}

resource "aws_s3_bucket_public_access_block" "media" {
  bucket = aws_s3_bucket.media.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# ACLs disabled: access is decided by IAM and the bucket policy only.
resource "aws_s3_bucket_ownership_controls" "media" {
  bucket = aws_s3_bucket.media.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

# SSE-S3 (AES256): matches the x-amz-server-side-encryption header the backend signs into
# every presigned PUT, and needs no KMS permissions or per-request KMS cost.
resource "aws_s3_bucket_server_side_encryption_configuration" "media" {
  bucket = aws_s3_bucket.media.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_versioning" "media" {
  bucket = aws_s3_bucket.media.id

  versioning_configuration {
    status = var.versioning ? "Enabled" : "Suspended"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "media" {
  bucket = aws_s3_bucket.media.id

  rule {
    id     = "abort-incomplete-multipart"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }

  rule {
    id     = "expire-old-versions"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = var.noncurrent_version_days
    }
  }

  depends_on = [aws_s3_bucket_versioning.media]
}

# The app (native) does not need CORS. The admin panel (Flutter Web, origins = admin_origins)
# loads signed GET URLs with XHR (location proofs, ad images) and PUTs ad images to presigned
# URLs, so GET/HEAD and PUT are allowed from those origins only. The bucket stays private:
# CORS grants no access by itself, every request still needs a valid presigned signature.
# The headers match what the backend signs: Content-Type, Content-Length,
# x-amz-server-side-encryption.
resource "aws_s3_bucket_cors_configuration" "media" {
  count = length(var.cors_allowed_origins) > 0 ? 1 : 0

  bucket = aws_s3_bucket.media.id

  cors_rule {
    allowed_methods = ["PUT", "GET", "HEAD"]
    allowed_origins = var.cors_allowed_origins
    allowed_headers = ["Content-Type", "Content-Length", "x-amz-server-side-encryption"]
    expose_headers  = ["ETag"]
    max_age_seconds = 3000
  }
}

locals {
  deny_insecure_transport = {
    Sid       = "DenyInsecureTransport"
    Effect    = "Deny"
    Principal = "*"
    Action    = "s3:*"
    Resource  = [aws_s3_bucket.media.arn, "${aws_s3_bucket.media.arn}/*"]
    Condition = { Bool = { "aws:SecureTransport" = "false" } }
  }

  # CloudFront may read ONLY the listed public-safe prefixes. Anything else (organiser
  # documents, certificates, chat images, profile photos of minors) stays reachable only
  # through short-lived presigned URLs issued after the backend's access check.
  cloudfront_read = var.enable_cdn ? [{
    Sid       = "AllowCloudFrontReadPublicPrefixes"
    Effect    = "Allow"
    Principal = { Service = "cloudfront.amazonaws.com" }
    Action    = "s3:GetObject"
    Resource  = [for p in var.cdn_public_prefixes : "${aws_s3_bucket.media.arn}/${trimsuffix(p, "/")}/*"]
    Condition = { StringEquals = { "AWS:SourceArn" = aws_cloudfront_distribution.media[0].arn } }
  }] : []
}

resource "aws_s3_bucket_policy" "media" {
  bucket = aws_s3_bucket.media.id

  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = concat([local.deny_insecure_transport], local.cloudfront_read)
  })

  depends_on = [aws_s3_bucket_public_access_block.media]
}

# ---------------------------------------------------------------- optional CloudFront (OAC)

resource "aws_cloudfront_origin_access_control" "media" {
  count = var.enable_cdn ? 1 : 0

  name                              = var.bucket_name
  description                       = "Athlix media bucket"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

resource "aws_cloudfront_distribution" "media" {
  count = var.enable_cdn ? 1 : 0

  enabled         = true
  comment         = "Athlix ${var.bucket_name} public media"
  is_ipv6_enabled = true
  price_class     = var.cdn_price_class

  origin {
    origin_id                = "media"
    domain_name              = aws_s3_bucket.media.bucket_regional_domain_name
    origin_access_control_id = aws_cloudfront_origin_access_control.media[0].id
  }

  default_cache_behavior {
    target_origin_id       = "media"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    compress               = true
    # AWS managed "CachingOptimized" policy.
    cache_policy_id = "658327ea-f89d-4fab-a63d-7e88639e58f6"
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }
}
