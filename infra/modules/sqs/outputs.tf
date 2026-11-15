output "queue_url" {
  description = "EVENTS_QUEUE_URL."
  value       = aws_sqs_queue.events.id
}

output "queue_arn" {
  value = aws_sqs_queue.events.arn
}

output "queue_name" {
  value = aws_sqs_queue.events.name
}

output "dlq_arn" {
  value = aws_sqs_queue.dlq.arn
}

output "dlq_name" {
  value = aws_sqs_queue.dlq.name
}

output "dlq_url" {
  value = aws_sqs_queue.dlq.id
}
