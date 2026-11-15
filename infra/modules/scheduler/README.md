# modules/scheduler

EventBridge Scheduler schedules that invoke the worker with `{"job":"<name>"}`. All
crons are in `Asia/Kolkata` time.

| Job | Schedule (IST) |
|---|---|
| `auth.cleanup_tokens` | daily 03:00 |
| `verification.minor_recheck` | daily 04:00 |
| `media.cleanup_pending` | hourly at :05 |
| `event.complete_past` | hourly at :10 |
| `listing.expire_orders` | every 15 minutes |
| `subscription.expire` | hourly at :20 |
| `notification.event_reminders` | hourly at :00 |
| `location.expire_residence` | daily 00:30 |
| `location.expiry_reminders` | daily 10:00 |
| `location.purge_proofs` | daily 02:30 |
| `admin.purge_login_events` | daily 03:15 |
| `verification.purge_subject_hash` | daily 03:45 |
| `ads.end_campaigns` | hourly at :25 |

It also creates a schedule group and a role Scheduler assumes (trust limited to this
account) with `lambda:InvokeFunction` on the worker only. Each schedule retries twice
within 15 minutes.

## Outputs

`schedule_group`, `jobs` (name to cron).

## Changing it safely

Job names must match the worker's job registry exactly. Use `disabled_jobs` to skip a
job temporarily instead of deleting it. Jobs should be idempotent: a retry may run one twice.
