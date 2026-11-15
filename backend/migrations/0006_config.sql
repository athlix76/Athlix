-- 0006: runtime configuration. Everything here is editable from the admin panel with no deploy.
-- Read through internal/platform/settings (Redis-cached, invalidated on admin save).

CREATE TABLE app_config (
  id            INT UNSIGNED NOT NULL AUTO_INCREMENT,
  config_key    VARCHAR(96)  NOT NULL,
  value         JSON         NOT NULL,
  value_type    ENUM('int','decimal','bool','string','string_list','json') NOT NULL,
  category      VARCHAR(32)  NOT NULL,
  description   VARCHAR(255) NOT NULL,
  is_public     TINYINT(1)   NOT NULL DEFAULT 0,
  version       INT UNSIGNED NOT NULL DEFAULT 1,
  updated_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_app_config_key (config_key),
  KEY idx_app_config_category (category)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Change history for every config-like table (app_config, listing_fee_rules, premium_plans,
-- premium_plan_benefits, policy_documents, verification_requirements, organiser_verification_steps, sports).
CREATE TABLE config_audit (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  table_name  VARCHAR(48)     NOT NULL,
  record_id   VARCHAR(96)     NOT NULL,
  old_value   JSON            NULL,
  new_value   JSON            NULL,
  changed_by  BIGINT UNSIGNED NOT NULL,
  changed_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_config_audit_record (table_name, record_id, changed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE policy_documents (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  doc_type        ENUM('terms','privacy','listing_refund','guardian_consent','community_guidelines','organiser_terms','entry_fee_disclaimer') NOT NULL,
  version         INT UNSIGNED    NOT NULL,
  title           VARCHAR(120)    NOT NULL,
  content_md      MEDIUMTEXT      NOT NULL,
  requires_acceptance TINYINT(1)  NOT NULL DEFAULT 0,
  active          TINYINT(1)      NOT NULL DEFAULT 0,
  effective_from  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by      BIGINT UNSIGNED NULL,
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_policy_version (doc_type, version),
  KEY idx_policy_active (doc_type, active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- What each role must complete, and which action it gates.
CREATE TABLE verification_requirements (
  id           INT UNSIGNED NOT NULL AUTO_INCREMENT,
  role         ENUM('player','coach','organiser') NOT NULL,
  requirement  ENUM('digilocker','guardian_consent_if_minor','organiser_approval','coach_pro_subscription') NOT NULL,
  is_required  TINYINT(1)   NOT NULL DEFAULT 1,
  gates        ENUM('app_access','create_event','offer_consultation','create_community') NOT NULL,
  updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_verification_req (role, requirement, gates)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE organiser_verification_steps (
  id           INT UNSIGNED NOT NULL AUTO_INCREMENT,
  step_code    VARCHAR(48)  NOT NULL,
  title        VARCHAR(120) NOT NULL,
  description  VARCHAR(500) NULL,
  input_type   ENUM('text','document','checkbox') NOT NULL,
  is_required  TINYINT(1)   NOT NULL DEFAULT 1,
  sort_order   INT          NOT NULL DEFAULT 0,
  active       TINYINT(1)   NOT NULL DEFAULT 1,
  updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_org_step_code (step_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

