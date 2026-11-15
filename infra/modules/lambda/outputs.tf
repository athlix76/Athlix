output "function_name" {
  value = aws_lambda_function.this.function_name
}

output "arn" {
  value = aws_lambda_function.this.arn
}

output "invoke_arn" {
  description = "ARN used by API Gateway integrations."
  value       = aws_lambda_function.this.invoke_arn
}

output "source_code_hash" {
  description = "Hash of the deployed zip (changes when the code changes)."
  value       = aws_lambda_function.this.source_code_hash
}

output "role_name" {
  value = aws_iam_role.this.name
}

output "role_arn" {
  value = aws_iam_role.this.arn
}

output "log_group_name" {
  value = aws_cloudwatch_log_group.this.name
}
