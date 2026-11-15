# One-time, per-account setup that the environment stacks rely on:
#   1. the S3 bucket and DynamoDB table that hold and lock Terraform state for dev and prod
#   2. the account-level API Gateway CloudWatch role (needed for WebSocket stage logging)
#   3. optionally, GitHub OIDC and a read-only "plan" role for the terraform-plan workflow
# This root keeps its own state locally (terraform.tfstate in this folder); see README.md.

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = "athlix"
      ManagedBy = "terraform"
      Stack     = "bootstrap"
    }
  }
}

data "aws_caller_identity" "current" {}

locals {
  account_id   = data.aws_caller_identity.current.account_id
  state_bucket = "athlix-tfstate-${local.account_id}"
  lock_table   = "athlix-tfstate-lock"
}

# ---------------------------------------------------------------- state bucket

resource "aws_s3_bucket" "state" {
  bucket = local.state_bucket

  # State holds the dev/prod database passwords; losing or deleting it is serious.
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket = aws_s3_bucket.state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    id     = "expire-old-state-versions"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }
}

resource "aws_s3_bucket_policy" "state" {
  bucket = aws_s3_bucket.state.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "DenyInsecureTransport"
      Effect    = "Deny"
      Principal = "*"
      Action    = "s3:*"
      Resource  = [aws_s3_bucket.state.arn, "${aws_s3_bucket.state.arn}/*"]
      Condition = { Bool = { "aws:SecureTransport" = "false" } }
    }]
  })

  depends_on = [aws_s3_bucket_public_access_block.state]
}

# ---------------------------------------------------------------- lock table

resource "aws_dynamodb_table" "lock" {
  name         = local.lock_table
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  point_in_time_recovery {
    enabled = true
  }

  server_side_encryption {
    enabled = true
  }

  lifecycle {
    prevent_destroy = true
  }
}

# ---------------------------------------------------------------- API Gateway logging role
# One per account and region. Without it, enabling WebSocket stage logging fails with
# "CloudWatch Logs role ARN must be set in account settings".

resource "aws_iam_role" "apigw_cloudwatch" {
  name        = "athlix-apigateway-cloudwatch"
  description = "Lets API Gateway write execution and access logs to CloudWatch"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "apigateway.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "apigw_cloudwatch" {
  role       = aws_iam_role.apigw_cloudwatch.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonAPIGatewayPushToCloudWatchLogs"
}

resource "aws_api_gateway_account" "this" {
  cloudwatch_role_arn = aws_iam_role.apigw_cloudwatch.arn

  depends_on = [aws_iam_role_policy_attachment.apigw_cloudwatch]
}

# ---------------------------------------------------------------- GitHub OIDC (optional)
# No access keys anywhere: the workflow exchanges its GitHub OIDC token for short-lived
# credentials of this role, and only on pull requests of the named repository.

locals {
  github_enabled = var.github_repo != ""
}

resource "aws_iam_openid_connect_provider" "github" {
  count = local.github_enabled && var.create_github_oidc_provider ? 1 : 0

  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]
  # AWS validates GitHub's certificate itself; these are GitHub's published thumbprints,
  # still required by the API.
  thumbprint_list = [
    "6938fd4d98bab03faadb97b34396831e3780aea1",
    "1c58a3a8518e8759bf075b76b750d4f2df264fcd",
  ]
}

locals {
  github_oidc_arn = local.github_enabled ? (
    var.create_github_oidc_provider
    ? aws_iam_openid_connect_provider.github[0].arn
    : "arn:aws:iam::${local.account_id}:oidc-provider/token.actions.githubusercontent.com"
  ) : ""
}

resource "aws_iam_role" "plan" {
  count = local.github_enabled ? 1 : 0

  name                 = "athlix-github-terraform-plan"
  description          = "terraform plan for infra/envs/dev from pull requests of ${var.github_repo}"
  max_session_duration = 3600

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = local.github_oidc_arn }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = { "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com" }
        StringLike   = { "token.actions.githubusercontent.com:sub" = "repo:${var.github_repo}:pull_request" }
      }
    }]
  })
}

# Read-only view of resources so plan can refresh them.
resource "aws_iam_role_policy_attachment" "plan_readonly" {
  count = local.github_enabled ? 1 : 0

  role       = aws_iam_role.plan[0].name
  policy_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
}

resource "aws_iam_role_policy" "plan" {
  count = local.github_enabled ? 1 : 0

  name = "terraform-plan-dev"
  role = aws_iam_role.plan[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "ReadDevState"
        Effect   = "Allow"
        Action   = ["s3:GetObject"]
        Resource = ["${aws_s3_bucket.state.arn}/athlix/dev/*"]
      },
      {
        Sid      = "ListState"
        Effect   = "Allow"
        Action   = ["s3:ListBucket"]
        Resource = [aws_s3_bucket.state.arn]
      },
      {
        Sid      = "StateLock"
        Effect   = "Allow"
        Action   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem", "dynamodb:DescribeTable"]
        Resource = [aws_dynamodb_table.lock.arn]
      },
      {
        # Refreshing aws_secretsmanager_secret_version.db reads the dev db secret. No other
        # secret has a Terraform-managed version, so no other value is readable.
        Sid      = "RefreshDevDbSecret"
        Effect   = "Allow"
        Action   = ["secretsmanager:GetSecretValue"]
        Resource = ["arn:aws:secretsmanager:${var.region}:${local.account_id}:secret:athlix/dev/db-*"]
      },
      {
        # ReadOnlyAccess also allows reading data. CI never needs user data, so take it away
        # explicitly: media objects (PII, KYC documents), queue messages, log contents,
        # prod state and every other secret value.
        Sid    = "DenyDataReads"
        Effect = "Deny"
        Action = [
          "s3:GetObject",
          "s3:GetObjectVersion",
        ]
        Resource = [
          "arn:aws:s3:::athlix-*-media-*/*",
          "${aws_s3_bucket.state.arn}/athlix/prod/*",
        ]
      },
      {
        Sid    = "DenyMessageAndLogReads"
        Effect = "Deny"
        Action = [
          "sqs:ReceiveMessage",
          "logs:GetLogEvents",
          "logs:FilterLogEvents",
          "logs:StartQuery",
          "logs:GetQueryResults",
          "dynamodb:Scan",
          "dynamodb:Query",
        ]
        Resource = ["*"]
      },
      {
        Sid         = "DenyOtherSecretValues"
        Effect      = "Deny"
        Action      = ["secretsmanager:GetSecretValue"]
        NotResource = ["arn:aws:secretsmanager:${var.region}:${local.account_id}:secret:athlix/dev/db-*"]
      },
    ]
  })
}
