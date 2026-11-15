resource "aws_sqs_queue" "dlq" {
  name                      = "${var.name}-dlq"
  message_retention_seconds = 1209600 # 14 days, the maximum: time to inspect and redrive
  sqs_managed_sse_enabled   = true
}

resource "aws_sqs_queue" "events" {
  name                       = var.name
  message_retention_seconds  = var.message_retention_seconds
  visibility_timeout_seconds = var.visibility_timeout_seconds
  receive_wait_time_seconds  = 20
  sqs_managed_sse_enabled    = true

  # After 5 failed receives a message moves to the DLQ (docs/architecture.md#deployment).
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dlq.arn
    maxReceiveCount     = var.max_receive_count
  })
}

resource "aws_sqs_queue_redrive_allow_policy" "dlq" {
  queue_url = aws_sqs_queue.dlq.id

  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue"
    sourceQueueArns   = [aws_sqs_queue.events.arn]
  })
}

# Worker trigger. ReportBatchItemFailures: the worker returns only the failed message ids,
# so one bad event is retried (and eventually dead-lettered) without replaying the batch.
resource "aws_lambda_event_source_mapping" "worker" {
  event_source_arn                   = aws_sqs_queue.events.arn
  function_name                      = var.worker_function_arn
  batch_size                         = var.batch_size
  maximum_batching_window_in_seconds = var.maximum_batching_window_seconds
  function_response_types            = ["ReportBatchItemFailures"]
  enabled                            = true

  scaling_config {
    maximum_concurrency = var.maximum_concurrency
  }
}
