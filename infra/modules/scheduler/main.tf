# One schedule per job in docs/architecture.md#scheduled-jobs. Job names must match exactly:
# the worker looks them up in the map returned by the modules' Jobs().
#
# Crons are evaluated in Asia/Kolkata (IST). Hourly jobs run at different minutes so they do
# not all hit RDS at once. Every job is idempotent, so a retried or skipped run is safe.
locals {
  jobs = {
    "auth.cleanup_tokens"             = "cron(0 3 * * ? *)"    # daily 03:00 IST
    "verification.minor_recheck"      = "cron(0 4 * * ? *)"    # daily 04:00 IST
    "media.cleanup_pending"           = "cron(5 * * * ? *)"    # hourly at :05
    "event.complete_past"             = "cron(10 * * * ? *)"   # hourly at :10
    "listing.expire_orders"           = "cron(0/15 * * * ? *)" # every 15 min
    "subscription.expire"             = "cron(20 * * * ? *)"   # hourly at :20
    "notification.event_reminders"    = "cron(0 * * * ? *)"    # hourly at :00
    "location.expire_residence"       = "cron(30 0 * * ? *)"   # daily 00:30 IST
    "location.expiry_reminders"       = "cron(0 10 * * ? *)"   # daily 10:00 IST
    "location.purge_proofs"           = "cron(30 2 * * ? *)"   # daily 02:30 IST (deletes location_proof/ objects)
    "admin.purge_login_events"        = "cron(15 3 * * ? *)"   # daily 03:15 IST (admin.login_events_retention_days)
    "verification.purge_subject_hash" = "cron(45 3 * * ? *)"   # daily 03:45 IST (verification.subject_hash_retention_days)
    "ads.end_campaigns"               = "cron(25 * * * ? *)"   # hourly at :25
  }

  enabled_jobs = { for k, v in local.jobs : k => v if !contains(var.disabled_jobs, k) }
}

resource "aws_scheduler_schedule_group" "this" {
  name = var.name
}

resource "aws_iam_role" "scheduler" {
  name        = "${var.name}-scheduler"
  description = "EventBridge Scheduler: invoke the ${var.name} worker only"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "scheduler.amazonaws.com" }
      Action    = "sts:AssumeRole"
      Condition = { StringEquals = { "aws:SourceAccount" = var.account_id } }
    }]
  })
}

resource "aws_iam_role_policy" "scheduler" {
  name = "invoke-worker"
  role = aws_iam_role.scheduler.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "InvokeWorker"
      Effect   = "Allow"
      Action   = ["lambda:InvokeFunction"]
      Resource = [var.worker_function_arn, "${var.worker_function_arn}:*"]
    }]
  })
}

resource "aws_scheduler_schedule" "job" {
  for_each = local.enabled_jobs

  # Schedule names allow letters, digits and - _ . only; dots are fine.
  name                         = each.key
  group_name                   = aws_scheduler_schedule_group.this.name
  description                  = "Athlix job ${each.key}"
  schedule_expression          = each.value
  schedule_expression_timezone = "Asia/Kolkata"
  state                        = "ENABLED"

  flexible_time_window {
    mode = "OFF"
  }

  target {
    arn      = var.worker_function_arn
    role_arn = aws_iam_role.scheduler.arn
    input    = jsonencode({ job = each.key })

    retry_policy {
      maximum_retry_attempts       = 2
      maximum_event_age_in_seconds = 900
    }
  }
}
