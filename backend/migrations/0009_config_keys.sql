-- 0009: config keys that were only code defaults before.
--   ads.max_per_request: sponsored placements returned per GET /ads (was a constant).
--   auth.otp.max_per_ip_per_hour: per-IP OTP limit (was the per-mobile value).
--   media.pending_cleanup_hours: age after which never-confirmed uploads are deleted.
INSERT INTO app_config (config_key, value, value_type, category, description, is_public) VALUES
('ads.max_per_request', '3', 'int', 'ads', 'Most sponsored placements returned by one GET /ads (1 to 10)', 0),
('auth.otp.max_per_ip_per_hour', '100', 'int', 'auth', 'OTP requests per client IP per hour (higher than per mobile: carrier CGNAT shares IPs)', 0),
('media.pending_cleanup_hours', '24', 'int', 'media', 'Uploads never confirmed within this many hours are deleted (object and row)', 0);
