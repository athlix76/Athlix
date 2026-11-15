output "state_bucket" {
  description = "Put this in infra/envs/<env>/backend.hcl as bucket."
  value       = aws_s3_bucket.state.bucket
}

output "lock_table" {
  description = "Put this in infra/envs/<env>/backend.hcl as dynamodb_table."
  value       = aws_dynamodb_table.lock.name
}

output "apigateway_cloudwatch_role_arn" {
  value = aws_iam_role.apigw_cloudwatch.arn
}

output "plan_role_arn" {
  description = "Store as the GitHub Actions secret AWS_PLAN_ROLE_ARN (empty when github_repo is not set)."
  value       = local.github_enabled ? aws_iam_role.plan[0].arn : ""
}
