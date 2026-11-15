resource "aws_db_subnet_group" "this" {
  name        = var.name
  description = "Private subnets for ${var.name} MySQL"
  subnet_ids  = var.private_subnet_ids
}

resource "aws_security_group" "db" {
  name        = "${var.name}-mysql"
  description = "MySQL: reachable only from the Lambda (and optional ops tunnel) security groups"
  vpc_id      = var.vpc_id
  tags        = { Name = "${var.name}-mysql" }
}

resource "aws_vpc_security_group_ingress_rule" "mysql" {
  count = length(var.allowed_security_group_ids)

  security_group_id            = aws_security_group.db.id
  description                  = "MySQL from an allowed security group"
  ip_protocol                  = "tcp"
  from_port                    = 3306
  to_port                      = 3306
  referenced_security_group_id = var.allowed_security_group_ids[count.index]
}

resource "aws_db_parameter_group" "this" {
  name        = "${var.name}-mysql80"
  family      = "mysql8.0"
  description = "Athlix MySQL 8.0 settings"

  # The backend connects with tls=preferred (always TLS against RDS), so refuse plain text.
  parameter {
    name  = "require_secure_transport"
    value = "1"
  }

  parameter {
    name  = "slow_query_log"
    value = "1"
  }

  parameter {
    name  = "long_query_time"
    value = "1"
  }

  # Do not log query text with bind values outside the slow log.
  parameter {
    name  = "general_log"
    value = "0"
  }
}

# Why not manage_master_user_password: the backend reads ONE secret named
# "<SECRET_PREFIX>/db" with the shape {"username","password","host","port","dbname"}
# (docs/security.md). The RDS-managed secret has a generated name, holds only username and
# password, and rotates on its own schedule, so it cannot be that secret. Instead a random
# password is generated here and written straight into the "db" secret container. It is
# never an output; it exists only in the RDS instance, Secrets Manager and the encrypted,
# access-restricted Terraform state bucket (see infra/README.md#state-holds-the-db-password).
resource "random_password" "master" {
  length  = 40
  special = false # alphanumeric only: no escaping problems in DSNs, CLIs or RDS rules
}

resource "aws_db_instance" "this" {
  identifier     = var.name
  engine         = "mysql"
  engine_version = var.engine_version
  instance_class = var.instance_class

  allocated_storage     = var.allocated_storage_gb
  max_allocated_storage = var.max_allocated_storage_gb
  storage_type          = "gp3"
  storage_encrypted     = true # AWS managed KMS key (aws/rds)

  db_name  = var.db_name
  username = var.master_username
  password = random_password.master.result
  port     = 3306

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.db.id]
  parameter_group_name   = aws_db_parameter_group.this.name
  publicly_accessible    = false
  multi_az               = var.multi_az

  backup_retention_period = var.backup_retention_days
  backup_window           = "19:30-20:30"         # UTC = 01:00-02:00 IST
  maintenance_window      = "sun:21:00-sun:22:00" # UTC = Mon 02:30-03:30 IST
  copy_tags_to_snapshot   = true

  auto_minor_version_upgrade = true
  apply_immediately          = var.apply_immediately

  deletion_protection       = var.deletion_protection
  skip_final_snapshot       = !var.final_snapshot
  final_snapshot_identifier = var.final_snapshot ? "${var.name}-final" : null

  performance_insights_enabled          = var.performance_insights_enabled
  performance_insights_retention_period = var.performance_insights_enabled ? 7 : null # 7 days is the free tier

  enabled_cloudwatch_logs_exports = var.cloudwatch_logs_exports

  tags = { Name = var.name }
}

# Writes the full "db" secret (shape from docs/security.md). The container itself is created
# by modules/secrets so every secret lives under one prefix.
resource "aws_secretsmanager_secret_version" "db" {
  secret_id = var.db_secret_id
  secret_string = jsonencode({
    username = aws_db_instance.this.username
    password = random_password.master.result
    host     = aws_db_instance.this.address
    port     = aws_db_instance.this.port
    dbname   = aws_db_instance.this.db_name
  })
}
