output "api_id" {
  value = aws_apigatewayv2_api.this.id
}

output "execute_api_url" {
  description = "Default endpoint, https://<id>.execute-api.<region>.amazonaws.com (no stage path: $default stage)."
  value       = aws_apigatewayv2_api.this.api_endpoint
}

output "base_url" {
  description = "Public base URL: the custom domain when set, otherwise the execute-api URL. The app uses <base_url>/v1."
  value       = var.domain_name == "" ? aws_apigatewayv2_api.this.api_endpoint : "https://${var.domain_name}"
}

output "custom_domain_target" {
  description = "Create a CNAME from domain_name to this host at your DNS provider (empty without a custom domain)."
  value       = var.domain_name == "" ? "" : aws_apigatewayv2_domain_name.this[0].domain_name_configuration[0].target_domain_name
}

output "access_log_group_name" {
  value = aws_cloudwatch_log_group.access.name
}
