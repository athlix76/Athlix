-- 0011: moderation and feed follow-ups.
--   reports.target_type: adds 'community' (a community is reported as itself, not as its conversation).
--   moderation.report_rate_limit_per_hour: reports one user may file per hour (was a constant).
--   feed.comment_max_length: longest comment body in characters (was a constant; capped at the column size).
ALTER TABLE reports
  MODIFY COLUMN target_type ENUM('user','event','post','comment','message','conversation','community') NOT NULL;

INSERT INTO app_config (config_key, value, value_type, category, description, is_public) VALUES
('moderation.report_rate_limit_per_hour', '20', 'int', 'moderation', 'Reports one user may file per hour', 0),
('feed.comment_max_length', '1000', 'int', 'feed', 'Longest comment body in characters (1 to 1000, the column size)', 1)
ON DUPLICATE KEY UPDATE config_key = config_key;
