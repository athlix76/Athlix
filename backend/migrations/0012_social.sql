-- 0012: social follow-ups (community, subscription).
--   conversations.removal_reason: reason given by an admin on POST /admin/communities/:id/remove.
--   community_ownership_transfers: audit row per POST /communities/:id/transfer-ownership.
--   premium.restore_max_purchases: most purchases one POST /premium/purchases/restore may carry.
ALTER TABLE conversations
  ADD COLUMN removal_reason VARCHAR(500) NULL;

CREATE TABLE community_ownership_transfers (
  id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  conversation_id  BIGINT UNSIGNED NOT NULL,
  from_user_id     BIGINT UNSIGNED NOT NULL,
  to_user_id       BIGINT UNSIGNED NOT NULL,
  created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_ownership_transfers_conversation (conversation_id, id),
  KEY idx_ownership_transfers_to_user (to_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO app_config (config_key, value, value_type, category, description, is_public) VALUES
('premium.restore_max_purchases', '20', 'int', 'premium', 'Most store purchases one POST /premium/purchases/restore call may re-verify (1 to 50)', 0);
