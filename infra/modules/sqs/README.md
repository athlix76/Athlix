# modules/sqs

Domain-events queue for the worker.

## What it creates

- `athlix-<env>-events` (SSE, 4-day retention, 720 s visibility = 6 x the worker
  timeout, 20 s long polling) with a redrive to `athlix-<env>-events-dlq` after 5
  receives (14-day retention).
- The worker event source mapping: batches of 10, `ReportBatchItemFailures` (only failed
  messages are retried), `maximum_concurrency` to cap DB load.

## Outputs

`queue_url` (`EVENTS_QUEUE_URL`), `queue_arn`, `queue_name`, `dlq_url`, `dlq_arn`,
`dlq_name`.

## Changing it safely

Keep the visibility timeout at least 6 x the worker timeout. Messages carry ids, not
PII. After fixing a bug, redrive the DLQ from the SQS console ("Start DLQ redrive").
