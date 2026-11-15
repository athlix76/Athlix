-- 0010: admin security and verification retention.
--   admin_login_events: one row per admin login attempt (success or failure). No password,
--     TOTP code or raw email is stored: the email is an HMAC (d.Crypto.Hash("admin_email")).
--   admin.login.max_per_ip / admin.login.window_minutes: the per-IP login rate limit (was a constant).
--   admin.password_min_length: password policy for POST /admin/me/password.
--   admin.login_events_retention_days: admin_login_events rows older than this are deleted.
--   verification.subject_hash_retention_days: days after account deletion that the identity
--     subject hash (and provider ref) is kept before the purge job clears it.

-- One row per admin login attempt. email_hash = HMAC of the lower-cased email; admin_id is NULL for unknown emails. Never holds passwords or TOTP codes.
CREATE TABLE IF NOT EXISTS admin_login_events (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  admin_id     BIGINT UNSIGNED NULL,
  email_hash   BINARY(32)      NOT NULL,
  ip           VARCHAR(45)     NULL,
  user_agent   VARCHAR(255)    NULL,
  outcome      ENUM('success','failure') NOT NULL,
  reason       VARCHAR(32)     NOT NULL,
  created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_admin_login_admin (admin_id, id),
  KEY idx_admin_login_email (email_hash, id),
  KEY idx_admin_login_outcome (outcome, id),
  KEY idx_admin_login_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO app_config (config_key, value, value_type, category, description, is_public) VALUES
('admin.login.max_per_ip', '20', 'int', 'admin', 'Admin login attempts per client IP per window (admin.login.window_minutes)', 0),
('admin.login.window_minutes', '15', 'int', 'admin', 'Window of the per-IP admin login rate limit, in minutes', 0),
('admin.password_min_length', '12', 'int', 'admin', 'Minimum admin password length (8 to 72) when an admin changes their password', 0),
('admin.login_events_retention_days', '365', 'int', 'admin', 'Admin login audit rows older than this many days are deleted', 0),
('verification.subject_hash_retention_days', '180', 'int', 'verification', 'Days after account deletion before the identity subject hash and provider ref are cleared', 0)
ON DUPLICATE KEY UPDATE config_key = config_key;
