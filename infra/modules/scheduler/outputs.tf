output "schedule_group" {
  value = aws_scheduler_schedule_group.this.name
}

output "jobs" {
  description = "Job name => schedule expression (IST) actually scheduled."
  value       = local.enabled_jobs
}
