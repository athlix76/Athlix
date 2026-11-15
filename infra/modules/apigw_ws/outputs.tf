output "api_id" {
  value = aws_apigatewayv2_api.this.id
}

output "execution_arn" {
  value = aws_apigatewayv2_api.this.execution_arn
}

output "management_url" {
  description = "WS_MANAGEMENT_URL: https://<id>.execute-api.<region>.amazonaws.com/<stage> (Lambdas post to connections here)."
  value       = local.management_url
}

output "public_url" {
  description = "WS_PUBLIC_URL returned by /chat/ws-ticket: the custom domain when set, otherwise the execute-api wss:// URL."
  value       = var.domain_name == "" ? "${aws_apigatewayv2_api.this.api_endpoint}/${var.stage_name}" : "wss://${var.domain_name}"
}

output "custom_domain_target" {
  value = var.domain_name == "" ? "" : aws_apigatewayv2_domain_name.this[0].domain_name_configuration[0].target_domain_name
}
