# modules/rds

MySQL 8.0 for one environment.

## What it creates

- Subnet group in the private subnets; security group allowing 3306 only from the given
  security groups (Lambda and ops).
- Parameter group: `require_secure_transport=1`, slow query log on (1 s), general log off.
- `aws_db_instance`: gp3, encrypted at rest, not publicly accessible, storage autoscaling,
  backups (window 01:00-02:00 IST), maintenance Monday 02:30-03:30 IST, optional
  Multi-AZ, deletion protection, final snapshot and Performance Insights. Exports error
  and slow-query logs (never the general log, which would contain query values).
- The master password from `random_password` (40 characters), written as a new version
  of the `athlix/<env>/db` secret in the shape the backend reads:
  `{"username","password","host","port","dbname"}`.

## Outputs

`instance_id`, `address`, `port`, `db_name`, `security_group_id`, `secret_version_id`.
The password is never an output (it does sit in state; see infra/README.md).

## Changing it safely

- `instance_class`, storage and Multi-AZ changes are in place (`apply_immediately` false
  by default: they wait for the maintenance window unless you set it).
- Changing `db_name`, `master_username` or the engine major version replaces the
  instance: do not, without a migration plan.
- `final_snapshot` must stay true in prod so a destroy keeps a copy.
