output "instance_id" {
  description = "Target for aws ssm start-session."
  value       = aws_instance.this.id
}
