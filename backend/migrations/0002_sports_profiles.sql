-- 0002: sports catalogue, profiles, stats, achievements, follows, blocks, organiser/coach data.

CREATE TABLE sports (
  id             INT UNSIGNED NOT NULL AUTO_INCREMENT,
  code           VARCHAR(32)  NOT NULL,
  name           VARCHAR(64)  NOT NULL,
  is_team_sport  TINYINT(1)   NOT NULL DEFAULT 0,
  icon_key       VARCHAR(255) NULL,
  active         TINYINT(1)   NOT NULL DEFAULT 1,
  sort_order     INT          NOT NULL DEFAULT 0,
  created_at     DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at     DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_sports_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE sport_positions (
  id          INT UNSIGNED NOT NULL AUTO_INCREMENT,
  sport_id    INT UNSIGNED NOT NULL,
  code        VARCHAR(32)  NOT NULL,
  name        VARCHAR(64)  NOT NULL,
  applies_to  ENUM('player','coach','both') NOT NULL DEFAULT 'player',
  active      TINYINT(1)   NOT NULL DEFAULT 1,
  sort_order  INT          NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY uq_sport_position (sport_id, code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- fields: [{"key":"runs","label":"Runs","type":"int","unit":null}, ...]
-- type is one of int | decimal | text | duration_ms | percent
CREATE TABLE stat_templates (
  id           INT UNSIGNED NOT NULL AUTO_INCREMENT,
  sport_id     INT UNSIGNED NOT NULL,
  position_id  INT UNSIGNED NULL,
  name         VARCHAR(64)  NOT NULL,
  fields       JSON         NOT NULL,
  version      INT          NOT NULL DEFAULT 1,
  active       TINYINT(1)   NOT NULL DEFAULT 1,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_stat_template_sport (sport_id, position_id, active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE profiles (
  user_id              BIGINT UNSIGNED NOT NULL,
  bio                  VARCHAR(500)    NULL,
  sport_id             INT UNSIGNED    NULL,
  position_id          INT UNSIGNED    NULL,
  city                 VARCHAR(64)     NULL,
  district             VARCHAR(64)     NULL,
  state                VARCHAR(64)     NULL,
  academy_club         VARCHAR(120)    NULL,
  photo_upload_id      BIGINT UNSIGNED NULL,
  birth_year           SMALLINT        NULL,
  visibility           ENUM('public','followers','private') NOT NULL DEFAULT 'public',
  stats_visibility     ENUM('public','followers','private') NOT NULL DEFAULT 'public',
  coaching_since_year  SMALLINT        NULL,
  followers_count      INT UNSIGNED    NOT NULL DEFAULT 0,
  following_count      INT UNSIGNED    NOT NULL DEFAULT 0,
  created_at           DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at           DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id),
  KEY idx_profiles_sport (sport_id, position_id),
  KEY idx_profiles_location (state, district, city),
  FULLTEXT KEY ft_profiles (bio, academy_club)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- birth_year above is set ONLY from verified DigiLocker DOB and powers age-group search filters.

CREATE TABLE profile_sports (
  user_id      BIGINT UNSIGNED NOT NULL,
  sport_id     INT UNSIGNED    NOT NULL,
  position_id  INT UNSIGNED    NULL,
  is_primary   TINYINT(1)      NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, sport_id),
  KEY idx_profile_sports_sport (sport_id, position_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE user_stats (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id      BIGINT UNSIGNED NOT NULL,
  sport_id     INT UNSIGNED    NOT NULL,
  template_id  INT UNSIGNED    NOT NULL,
  season       VARCHAR(16)     NOT NULL,
  stats        JSON            NOT NULL,
  created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_user_stats_season (user_id, sport_id, season),
  KEY idx_user_stats_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE achievements (
  id                       BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id                  BIGINT UNSIGNED NOT NULL,
  sport_id                 INT UNSIGNED    NULL,
  title                    VARCHAR(120)    NOT NULL,
  description              VARCHAR(500)    NULL,
  level                    ENUM('school','college','club','district','state','national','international','other') NOT NULL DEFAULT 'other',
  achieved_on              DATE            NULL,
  certificate_upload_id    BIGINT UNSIGNED NULL,
  created_at               DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at               DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_achievements_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE follows (
  follower_id  BIGINT UNSIGNED NOT NULL,
  followee_id  BIGINT UNSIGNED NOT NULL,
  created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (follower_id, followee_id),
  KEY idx_follows_followee (followee_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE blocks (
  blocker_id  BIGINT UNSIGNED NOT NULL,
  blocked_id  BIGINT UNSIGNED NOT NULL,
  created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (blocker_id, blocked_id),
  KEY idx_blocks_blocked (blocked_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- One row per (viewer, viewed, day). Powers the see_profile_viewers premium benefit.
-- Views by minors are never recorded.
CREATE TABLE profile_views (
  viewed_id   BIGINT UNSIGNED NOT NULL,
  viewer_id   BIGINT UNSIGNED NOT NULL,
  viewed_on   DATE            NOT NULL,
  created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (viewed_id, viewed_on, viewer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE organiser_profiles (
  user_id         BIGINT UNSIGNED NOT NULL,
  org_name        VARCHAR(120)    NULL,
  org_type        ENUM('individual','club','academy','school','college','association','company','other') NULL,
  status          ENUM('not_started','draft','submitted','approved','rejected') NOT NULL DEFAULT 'not_started',
  reviewed_by     BIGINT UNSIGNED NULL,
  review_note     VARCHAR(500)    NULL,
  submitted_at    DATETIME(3)     NULL,
  reviewed_at     DATETIME(3)     NULL,
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id),
  KEY idx_organiser_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE organiser_verification_submissions (
  id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id             BIGINT UNSIGNED NOT NULL,
  step_id             INT UNSIGNED    NOT NULL,
  value_text          VARCHAR(500)    NULL,
  document_upload_id  BIGINT UNSIGNED NULL,
  status              ENUM('submitted','approved','rejected') NOT NULL DEFAULT 'submitted',
  reviewed_by         BIGINT UNSIGNED NULL,
  review_note         VARCHAR(500)    NULL,
  created_at          DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  reviewed_at         DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_org_submission (user_id, step_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Coach consultation listing (payments happen directly between player and coach).
CREATE TABLE coach_consultations (
  user_id      BIGINT UNSIGNED NOT NULL,
  offered      TINYINT(1)      NOT NULL DEFAULT 0,
  description  VARCHAR(1000)   NULL,
  fee_note     VARCHAR(200)    NULL,
  modes        JSON            NOT NULL,
  updated_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE consultation_requests (
  id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  coach_id         BIGINT UNSIGNED NOT NULL,
  player_id        BIGINT UNSIGNED NOT NULL,
  message          VARCHAR(1000)   NOT NULL,
  preferred_mode   ENUM('online','in_person') NOT NULL,
  status           ENUM('pending','accepted','declined','cancelled','completed') NOT NULL DEFAULT 'pending',
  created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_consult_coach (coach_id, status),
  KEY idx_consult_player (player_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
