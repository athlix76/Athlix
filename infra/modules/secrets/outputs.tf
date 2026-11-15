# Names and ARNs only. Secret values are never outputs.

output "arns" {
  description = "Map of short name => secret ARN (use these exact ARNs in IAM, never a wildcard)."
  value       = { for k, s in aws_secretsmanager_secret.this : k => s.arn }
}

output "names" {
  description = "Map of short name => full secret name."
  value       = { for k, s in aws_secretsmanager_secret.this : k => s.name }
}

output "prefix" {
  value = var.prefix
}
