data "aws_region" "current" {}

locals {
  # $connect / $disconnect plus the client actions from docs/api.md#websocket-protocol.
  # route_selection_expression is $request.body.action, so {"action":"sendMessage"} hits the
  # sendMessage route and anything unknown falls to $default. All go to the same ws Lambda.
  message_routes = ["$default", "sendMessage", "typing", "markRead", "ping"]
  all_routes     = concat(["$connect", "$disconnect"], local.message_routes)

  management_url = "https://${aws_apigatewayv2_api.this.id}.execute-api.${data.aws_region.current.name}.amazonaws.com/${var.stage_name}"
}

resource "aws_apigatewayv2_api" "this" {
  name                       = var.name
  description                = "Athlix chat WebSocket API"
  protocol_type              = "WEBSOCKET"
  route_selection_expression = "$request.body.action"
}

resource "aws_apigatewayv2_integration" "lambda" {
  api_id                    = aws_apigatewayv2_api.this.id
  integration_type          = "AWS_PROXY"
  integration_method        = "POST"
  integration_uri           = var.ws_lambda_invoke_arn
  content_handling_strategy = "CONVERT_TO_TEXT"
  passthrough_behavior      = "WHEN_NO_MATCH"
}

# No API Gateway authorizer on $connect: the client passes a single-use ticket
# (?ticket=..., from POST /v1/chat/ws-ticket) and the ws Lambda redeems it in Redis and
# refuses the connection otherwise (docs/architecture.md#real-time-chat).
resource "aws_apigatewayv2_route" "this" {
  for_each = toset(local.all_routes)

  api_id                              = aws_apigatewayv2_api.this.id
  route_key                           = each.key
  authorization_type                  = "NONE"
  target                              = "integrations/${aws_apigatewayv2_integration.lambda.id}"
  route_response_selection_expression = contains(local.message_routes, each.key) ? "$default" : null
}

# Two-way routes: whatever the Lambda returns as the body (e.g. a pong) goes back to the
# caller. Pushes to other members use the management API instead.
resource "aws_apigatewayv2_route_response" "this" {
  for_each = toset(local.message_routes)

  api_id             = aws_apigatewayv2_api.this.id
  route_id           = aws_apigatewayv2_route.this[each.key].id
  route_response_key = "$default"
}

resource "aws_apigatewayv2_deployment" "this" {
  api_id      = aws_apigatewayv2_api.this.id
  description = "Managed by Terraform"

  triggers = {
    redeployment = sha1(jsonencode([
      aws_apigatewayv2_integration.lambda.id,
      [for k in sort(local.all_routes) : aws_apigatewayv2_route.this[k].id],
      [for k in sort(local.message_routes) : aws_apigatewayv2_route_response.this[k].id],
    ]))
  }

  lifecycle {
    create_before_destroy = true
  }

  depends_on = [
    aws_apigatewayv2_route.this,
    aws_apigatewayv2_route_response.this,
  ]
}

resource "aws_cloudwatch_log_group" "access" {
  count = var.enable_access_logs ? 1 : 0

  name              = "/aws/apigateway/${var.name}"
  retention_in_days = var.log_retention_days
}

resource "aws_apigatewayv2_stage" "this" {
  api_id        = aws_apigatewayv2_api.this.id
  name          = var.stage_name
  deployment_id = aws_apigatewayv2_deployment.this.id

  default_route_settings {
    throttling_burst_limit = var.throttling_burst_limit
    throttling_rate_limit  = var.throttling_rate_limit
    data_trace_enabled     = false
    logging_level          = var.enable_access_logs ? "ERROR" : "OFF"
  }

  # WebSocket logging needs the account-level API Gateway CloudWatch role (infra/bootstrap).
  # Fields: no headers, no query string (the $connect ticket travels there), no frames.
  dynamic "access_log_settings" {
    for_each = var.enable_access_logs ? [1] : []
    content {
      destination_arn = aws_cloudwatch_log_group.access[0].arn
      format = jsonencode({
        requestId        = "$context.requestId"
        requestTime      = "$context.requestTime"
        ip               = "$context.identity.sourceIp"
        eventType        = "$context.eventType"
        routeKey         = "$context.routeKey"
        connectionId     = "$context.connectionId"
        status           = "$context.status"
        integrationError = "$context.integrationErrorMessage"
      })
    }
  }
}

resource "aws_lambda_permission" "apigw" {
  statement_id  = "AllowWebSocketApiInvoke"
  action        = "lambda:InvokeFunction"
  function_name = var.ws_lambda_function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.this.execution_arn}/*"
}

# api (pushes after REST sends), ws (fan-out after sendMessage) and worker (event handlers)
# may post to / drop connections of THIS API's stage only.
resource "aws_iam_role_policy" "manage_connections" {
  count = length(var.manage_connections_role_names)

  name = "${var.name}-manage-connections"
  role = var.manage_connections_role_names[count.index]

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "ManageWebSocketConnections"
      Effect   = "Allow"
      Action   = ["execute-api:ManageConnections"]
      Resource = ["${aws_apigatewayv2_api.this.execution_arn}/${var.stage_name}/*"]
    }]
  })
}

# ---------------------------------------------------------------- optional custom domain

resource "aws_apigatewayv2_domain_name" "this" {
  count = var.domain_name == "" ? 0 : 1

  domain_name = var.domain_name

  domain_name_configuration {
    certificate_arn = var.certificate_arn
    endpoint_type   = "REGIONAL"
    security_policy = "TLS_1_2"
  }
}

resource "aws_apigatewayv2_api_mapping" "this" {
  count = var.domain_name == "" ? 0 : 1

  api_id      = aws_apigatewayv2_api.this.id
  domain_name = aws_apigatewayv2_domain_name.this[0].id
  stage       = aws_apigatewayv2_stage.this.id
}
