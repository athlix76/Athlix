-- 0005: premium plans + subscriptions, ads, moderation, admin users/roles, audit.

CREATE TABLE premium_plans (
  id                       INT UNSIGNED NOT NULL AUTO_INCREMENT,
  code                     VARCHAR(48)  NOT NULL,
  audience                 ENUM('player','coach','organiser') NOT NULL,
  name                     VARCHAR(80)  NOT NULL,
  description              VARCHAR(500) NULL,
  price_paise              INT UNSIGNED NOT NULL,
  billing_period           ENUM('monthly','quarterly','yearly') NOT NULL,
  store_product_id_android VARCHAR(128) NULL,
  store_product_id_ios     VARCHAR(128) NULL,
  active                   TINYINT(1)   NOT NULL DEFAULT 1,
  sort_order               INT          NOT NULL DEFAULT 0,
  created_at               DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at               DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_premium_plan_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- benefit_key values are a closed list in docs/config-system.md (e.g. consultation_listing,
-- profile_boost, advanced_stats, see_profile_viewers, extra_communities, verified_coach_badge).
CREATE TABLE premium_plan_benefits (
  id           INT UNSIGNED NOT NULL AUTO_INCREMENT,
  plan_id      INT UNSIGNED NOT NULL,
  benefit_key  VARCHAR(48)  NOT NULL,
  value        JSON         NOT NULL,
  label        VARCHAR(120) NOT NULL,
  sort_order   INT          NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uq_plan_benefit (plan_id, benefit_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE subscriptions (
  id                     BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id                BIGINT UNSIGNED NOT NULL,
  plan_id                INT UNSIGNED    NOT NULL,
  store                  ENUM('google_play','app_store','manual') NOT NULL,
  store_transaction_id   VARCHAR(255)    NULL,
  purchase_token_hash    BINARY(32)      NULL,
  status                 ENUM('active','grace','expired','cancelled','refunded') NOT NULL,
  auto_renew             TINYINT(1)      NOT NULL DEFAULT 1,
  started_at             DATETIME(3)     NOT NULL,
  expires_at             DATETIME(3)     NOT NULL,
  created_at             DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at             DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_subscription_store_txn (store, store_transaction_id),
  KEY idx_subscriptions_user (user_id, status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE ad_campaigns (
  id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  advertiser_name  VARCHAR(120)    NOT NULL,
  title            VARCHAR(120)    NOT NULL,
  body             VARCHAR(300)    NULL,
  image_upload_id  BIGINT UNSIGNED NULL,
  target_url       VARCHAR(500)    NOT NULL,
  placement        ENUM('home_feed','events_list','search','event_detail') NOT NULL,
  sport_id         INT UNSIGNED    NULL,
  state            VARCHAR(64)     NULL,
  priority         INT             NOT NULL DEFAULT 0,
  starts_at        DATETIME(3)     NOT NULL,
  ends_at          DATETIME(3)     NOT NULL,
  status           ENUM('draft','active','paused','ended') NOT NULL DEFAULT 'draft',
  created_by       BIGINT UNSIGNED NULL,
  created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_ads_serving (status, placement, starts_at, ends_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE ad_daily_stats (
  ad_id        BIGINT UNSIGNED NOT NULL,
  stat_date    DATE            NOT NULL,
  impressions  INT UNSIGNED    NOT NULL DEFAULT 0,
  clicks       INT UNSIGNED    NOT NULL DEFAULT 0,
  PRIMARY KEY (ad_id, stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE reports (
  id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  reporter_id      BIGINT UNSIGNED NOT NULL,
  target_type      ENUM('user','event','post','comment','message','conversation') NOT NULL,
  target_id        BIGINT UNSIGNED NOT NULL,
  reason           ENUM('spam','fake_profile','harassment','scam','inappropriate','underage_safety','payment_issue','other') NOT NULL,
  details          VARCHAR(1000)   NULL,
  status           ENUM('open','actioned','dismissed') NOT NULL DEFAULT 'open',
  resolved_by      BIGINT UNSIGNED NULL,
  resolution_note  VARCHAR(500)    NULL,
  created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  resolved_at      DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_reports_once (reporter_id, target_type, target_id),
  KEY idx_reports_target (target_type, target_id, status),
  KEY idx_reports_status (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE moderation_actions (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  admin_id     BIGINT UNSIGNED NULL,
  target_type  ENUM('user','event','post','comment','message','conversation') NOT NULL,
  target_id    BIGINT UNSIGNED NOT NULL,
  action       ENUM('hide','unhide','suspend_user','unsuspend_user','remove','warn','auto_hide') NOT NULL,
  reason       VARCHAR(500)    NULL,
  report_id    BIGINT UNSIGNED NULL,
  created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_moderation_target (target_type, target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- permissions: JSON array of permission strings (closed list in docs/security.md).
CREATE TABLE admin_roles (
  id           INT UNSIGNED NOT NULL AUTO_INCREMENT,
  name         VARCHAR(48)  NOT NULL,
  description  VARCHAR(255) NULL,
  permissions  JSON         NOT NULL,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_admin_roles_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE admin_users (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  email             VARCHAR(190)    NOT NULL,
  name              VARCHAR(80)     NOT NULL,
  password_hash     VARCHAR(255)    NOT NULL,
  totp_secret_enc   VARBINARY(255)  NULL,
  key_version       SMALLINT UNSIGNED NULL,
  role_id           INT UNSIGNED    NOT NULL,
  status            ENUM('active','disabled') NOT NULL DEFAULT 'active',
  failed_logins     INT UNSIGNED    NOT NULL DEFAULT 0,
  locked_until      DATETIME(3)     NULL,
  last_login_at     DATETIME(3)     NULL,
  created_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_admin_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE admin_audit_log (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  admin_id     BIGINT UNSIGNED NOT NULL,
  action       VARCHAR(64)     NOT NULL,
  target_type  VARCHAR(32)     NULL,
  target_id    VARCHAR(64)     NULL,
  details      JSON            NULL,
  ip           VARCHAR(45)     NULL,
  created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_audit_admin (admin_id, created_at),
  KEY idx_audit_target (target_type, target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
