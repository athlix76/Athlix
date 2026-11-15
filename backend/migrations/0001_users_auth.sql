-- 0001: users, roles, auth tokens, guardians, identity verification, media uploads.
-- Conventions: utf8mb4, BIGINT UNSIGNED ids, money in paise (INT), times in UTC DATETIME(3).
-- Sensitive PII columns end in _enc (AES-256-GCM, see internal/platform/crypto) and have a
-- matching _hash (HMAC-SHA256) when lookup is needed, plus _last4 for masked display.

CREATE TABLE users (
  id                       BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  mobile_enc               VARBINARY(255)  NOT NULL,
  mobile_hash              BINARY(32)      NOT NULL,
  mobile_last4             CHAR(4)         NOT NULL,
  display_name             VARCHAR(80)     NULL,
  primary_role             ENUM('player','coach','organiser') NULL,
  status                   ENUM('active','suspended','deleted') NOT NULL DEFAULT 'active',
  onboarding_step          ENUM('role','verification','guardian','profile','done') NOT NULL DEFAULT 'role',
  verification_status      ENUM('unverified','pending','verified','failed') NOT NULL DEFAULT 'unverified',
  is_minor                 TINYINT(1)      NOT NULL DEFAULT 0,
  guardian_consent_status  ENUM('not_required','pending','granted','revoked') NOT NULL DEFAULT 'not_required',
  suspended_reason         VARCHAR(255)    NULL,
  last_login_at            DATETIME(3)     NULL,
  created_at               DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at               DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at               DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_users_mobile_hash (mobile_hash),
  KEY idx_users_status (status),
  KEY idx_users_minor (is_minor)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE user_roles (
  user_id     BIGINT UNSIGNED NOT NULL,
  role        ENUM('player','coach','organiser') NOT NULL,
  created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id, role),
  KEY idx_user_roles_role (role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Refresh tokens for both app users and admins. Only the SHA-256 hash of the token is stored.
CREATE TABLE refresh_tokens (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  subject_type  ENUM('user','admin') NOT NULL,
  subject_id    BIGINT UNSIGNED NOT NULL,
  token_hash    BINARY(32)      NOT NULL,
  family_id     CHAR(36)        NOT NULL,
  device_id     VARCHAR(64)     NULL,
  expires_at    DATETIME(3)     NOT NULL,
  revoked_at    DATETIME(3)     NULL,
  replaced_by   BIGINT UNSIGNED NULL,
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_refresh_token_hash (token_hash),
  KEY idx_refresh_subject (subject_type, subject_id),
  KEY idx_refresh_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE guardian_consents (
  id                     BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id                BIGINT UNSIGNED NOT NULL,
  guardian_name_enc      VARBINARY(512)  NOT NULL,
  guardian_mobile_enc    VARBINARY(255)  NOT NULL,
  guardian_mobile_hash   BINARY(32)      NOT NULL,
  guardian_mobile_last4  CHAR(4)         NOT NULL,
  relation               ENUM('mother','father','legal_guardian','other') NOT NULL,
  status                 ENUM('pending','granted','revoked','expired') NOT NULL DEFAULT 'pending',
  policy_document_id     BIGINT UNSIGNED NOT NULL,
  key_version            SMALLINT UNSIGNED NOT NULL,
  granted_at             DATETIME(3)     NULL,
  revoked_at             DATETIME(3)     NULL,
  revoked_reason         VARCHAR(255)    NULL,
  created_at             DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at             DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_guardian_user (user_id, status),
  KEY idx_guardian_mobile (guardian_mobile_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- One row per verification attempt. The full Aadhaar number, XML, PDF and photo are NEVER stored.
-- subject_hash = HMAC of the provider's stable subject id (e.g. DigiLocker id) to stop one
-- identity creating many accounts.
CREATE TABLE identity_verifications (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id         BIGINT UNSIGNED NOT NULL,
  provider        VARCHAR(32)     NOT NULL,
  provider_ref    VARCHAR(128)    NULL,
  status          ENUM('initiated','verified','failed','expired') NOT NULL DEFAULT 'initiated',
  subject_hash    BINARY(32)      NULL,
  aadhaar_last4   CHAR(4)         NULL,
  name_enc        VARBINARY(512)  NULL,
  dob_enc         VARBINARY(128)  NULL,
  gender          ENUM('M','F','T','U') NULL,
  key_version     SMALLINT UNSIGNED NULL,
  failure_reason  VARCHAR(64)     NULL,
  verified_at     DATETIME(3)     NULL,
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_identity_subject (subject_hash),
  KEY idx_identity_user (user_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Every file a client uploads. The backend builds s3_key; clients only ever see upload ids.
CREATE TABLE media_uploads (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id       BIGINT UNSIGNED NOT NULL,
  purpose       ENUM('profile_photo','achievement_certificate','event_banner','chat_image','post_image','community_avatar','organiser_document') NOT NULL,
  s3_key        VARCHAR(255)    NOT NULL,
  content_type  VARCHAR(64)     NOT NULL,
  size_bytes    INT UNSIGNED    NOT NULL,
  status        ENUM('pending','confirmed','attached','deleted') NOT NULL DEFAULT 'pending',
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  confirmed_at  DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_media_key (s3_key),
  KEY idx_media_user (user_id, purpose, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE user_settings (
  user_id             BIGINT UNSIGNED NOT NULL,
  notification_prefs  JSON NOT NULL,
  privacy             JSON NOT NULL,
  updated_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE policy_acceptances (
  id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id             BIGINT UNSIGNED NOT NULL,
  policy_document_id  BIGINT UNSIGNED NOT NULL,
  accepted_at         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_policy_accept (user_id, policy_document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
