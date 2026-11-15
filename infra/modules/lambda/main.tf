resource "aws_cloudwatch_log_group" "this" {
  name              = "/aws/lambda/${var.function_name}"
  retention_in_days = var.log_retention_days
}

resource "aws_iam_role" "this" {
  name        = var.function_name
  description = "Execution role of the ${var.function_name} Lambda"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

locals {
  # Every statement has the same shape so the list stays one type; empty ones are dropped.
  statements = [
    for s in concat(
      [
        {
          Sid      = "OwnLogGroup"
          Effect   = "Allow"
          Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
          Resource = ["${aws_cloudwatch_log_group.this.arn}:*"]
        },
        {
          # Required for VPC Lambdas (same actions as AWSLambdaVPCAccessExecutionRole).
          # AWS does not support resource-level scoping for these.
          Sid    = "VpcNetworkInterfaces"
          Effect = "Allow"
          Action = [
            "ec2:CreateNetworkInterface",
            "ec2:DescribeNetworkInterfaces",
            "ec2:DescribeSubnets",
            "ec2:DeleteNetworkInterface",
            "ec2:AssignPrivateIpAddresses",
            "ec2:UnassignPrivateIpAddresses",
          ]
          Resource = ["*"]
        },
        {
          # Only the exact secret ARNs this function needs (never athlix/* or *).
          Sid      = "ReadOwnSecrets"
          Effect   = "Allow"
          Action   = ["secretsmanager:GetSecretValue"]
          Resource = var.secret_arns
        },
      ],
      [
        for st in var.policy_statements : {
          Sid      = st.sid
          Effect   = "Allow"
          Action   = st.actions
          Resource = st.resources
        }
      ],
    ) : s if length(s.Resource) > 0
  ]
}

resource "aws_iam_role_policy" "this" {
  name = "least-privilege"
  role = aws_iam_role.this.id

  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = local.statements
  })
}

resource "aws_lambda_function" "this" {
  function_name = var.function_name
  description   = var.description
  role          = aws_iam_role.this.arn

  filename         = var.zip_path
  source_code_hash = filebase64sha256(var.zip_path)
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]

  memory_size                    = var.memory_mb
  timeout                        = var.timeout_seconds
  reserved_concurrent_executions = var.reserved_concurrency

  vpc_config {
    subnet_ids         = var.subnet_ids
    security_group_ids = var.security_group_ids
  }

  environment {
    variables = var.environment
  }

  # The policy must exist before the function: an SQS event source mapping is validated
  # against the role's permissions at creation time.
  depends_on = [
    aws_iam_role_policy.this,
    aws_cloudwatch_log_group.this,
  ]
}
