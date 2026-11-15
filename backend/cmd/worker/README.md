# worker

The Lambda that consumes the domain-event queue and runs scheduled jobs. One binary, two
invocation shapes.

## SQS (domain events)

- Trigger: the events queue (`EVENTS_QUEUE_URL`), with **ReportBatchItemFailures** turned on.
- Each record body is a JSON `events.Event` (`{type, actorId, payload, occurredAt}`).
- Every module's `EventHandlers()` is registered on `d.Dispatcher` through
  `app.RegisterEventHandlers(d, modules.All(d))`, and each record is dispatched to all handlers
  for its type.
- A record whose handler fails is returned in `batchItemFailures`, so only that record is
  redelivered (and goes to the DLQ after the queue's max receives). Handlers are idempotent.
- A body that is not an Event is logged by message id and dropped, because a retry can never
  succeed.

## EventBridge (scheduled jobs)

- Rule input `{"job":"<name>"}` (or an EventBridge event with `detail.job`).
- Jobs come from every module's `Jobs()` through `app.CollectJobs(modules.All(d))`, so a new
  module's jobs (for example `notification.event_reminders`, `location.expire_residence`,
  `location.expiry_reminders`, `location.purge_proofs`) need no change here.
- An unknown job name or a failing job returns an error, so the invocation shows as failed.

## Local use

```
make worker-job JOB=notification.event_reminders   # go run ./cmd/worker -job <name>
go run ./cmd/worker -list                          # print the known job names
```

Locally events are delivered in-process by the api, so there is no queue to consume.

## Logging

Message ids, event types, job names and error text only. Never event payloads or bodies.

## Tests

`go test ./cmd/worker/`: partial batch failures, malformed bodies dropped, job payload shapes,
unknown and failing jobs, and jobs collected from several modules.
