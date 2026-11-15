# The password is deliberately NOT an output. Read it from Secrets Manager if you must.

output "instance_id" {
  value = aws_db_instance.this.identifier
}

output "address" {
  description = "Private DNS name of the instance (resolves only inside the VPC)."
  value       = aws_db_instance.this.address
}

output "port" {
  value = aws_db_instance.this.port
}

output "db_name" {
  value = aws_db_instance.this.db_name
}

output "security_group_id" {
  value = aws_security_group.db.id
}

output "secret_version_id" {
  description = "Version id of the db secret value, so dependants (migrate) wait until it is written."
  value       = aws_secretsmanager_secret_version.db.version_id
}
