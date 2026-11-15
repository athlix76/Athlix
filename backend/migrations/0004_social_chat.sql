-- 0004: posts/feed, conversations (direct, community, event group), messages, notifications, devices.

CREATE TABLE posts (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  author_id       BIGINT UNSIGNED NOT NULL,
  body            VARCHAR(2000)   NULL,
  media_upload_ids JSON           NOT NULL,
  sport_id        INT UNSIGNED    NULL,
  event_id        BIGINT UNSIGNED NULL,
  visibility      ENUM('public','followers') NOT NULL DEFAULT 'public',
  like_count      INT UNSIGNED    NOT NULL DEFAULT 0,
  comment_count   INT UNSIGNED    NOT NULL DEFAULT 0,
  status          ENUM('visible','hidden','deleted') NOT NULL DEFAULT 'visible',
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_posts_author (author_id, status, id),
  KEY idx_posts_recent (status, visibility, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE post_likes (
  post_id     BIGINT UNSIGNED NOT NULL,
  user_id     BIGINT UNSIGNED NOT NULL,
  created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (post_id, user_id),
  KEY idx_post_likes_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE post_comments (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  post_id     BIGINT UNSIGNED NOT NULL,
  author_id   BIGINT UNSIGNED NOT NULL,
  body        VARCHAR(1000)   NOT NULL,
  status      ENUM('visible','hidden','deleted') NOT NULL DEFAULT 'visible',
  created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_post_comments_post (post_id, status, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- type=direct: exactly 2 members, direct_key = "<minUserId>:<maxUserId>".
-- type=community: user-created group. type=event: auto-created group for a published event.
CREATE TABLE conversations (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  type              ENUM('direct','community','event') NOT NULL,
  title             VARCHAR(80)     NULL,
  description       VARCHAR(500)    NULL,
  avatar_upload_id  BIGINT UNSIGNED NULL,
  owner_id          BIGINT UNSIGNED NULL,
  event_id          BIGINT UNSIGNED NULL,
  sport_id          INT UNSIGNED    NULL,
  city              VARCHAR(64)     NULL,
  state             VARCHAR(64)     NULL,
  visibility        ENUM('public','private') NOT NULL DEFAULT 'private',
  join_policy       ENUM('open','approval','invite') NOT NULL DEFAULT 'invite',
  direct_key        VARCHAR(48)     NULL,
  member_count      INT UNSIGNED    NOT NULL DEFAULT 0,
  last_message_id   BIGINT UNSIGNED NULL,
  last_message_at   DATETIME(3)     NULL,
  status            ENUM('active','archived','removed') NOT NULL DEFAULT 'active',
  created_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_conversations_direct (direct_key),
  UNIQUE KEY uq_conversations_event (event_id),
  KEY idx_conversations_discover (type, visibility, status, sport_id),
  FULLTEXT KEY ft_conversations (title, description)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE conversation_members (
  conversation_id       BIGINT UNSIGNED NOT NULL,
  user_id               BIGINT UNSIGNED NOT NULL,
  role                  ENUM('owner','admin','member') NOT NULL DEFAULT 'member',
  status                ENUM('active','pending','invited','left','removed','banned') NOT NULL DEFAULT 'active',
  muted_until           DATETIME(3)     NULL,
  last_read_message_id  BIGINT UNSIGNED NULL,
  joined_at             DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at            DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (conversation_id, user_id),
  KEY idx_members_user (user_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE messages (
  id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  conversation_id  BIGINT UNSIGNED NOT NULL,
  sender_id        BIGINT UNSIGNED NULL,
  kind             ENUM('text','image','system') NOT NULL DEFAULT 'text',
  body             TEXT            NULL,
  media_upload_id  BIGINT UNSIGNED NULL,
  reply_to_id      BIGINT UNSIGNED NULL,
  client_msg_id    VARCHAR(64)     NULL,
  status           ENUM('visible','deleted','hidden') NOT NULL DEFAULT 'visible',
  created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  edited_at        DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_messages_client (sender_id, client_msg_id),
  KEY idx_messages_conversation (conversation_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE notifications (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id     BIGINT UNSIGNED NOT NULL,
  type        VARCHAR(48)     NOT NULL,
  title       VARCHAR(120)    NOT NULL,
  body        VARCHAR(500)    NOT NULL,
  data        JSON            NOT NULL,
  read_at     DATETIME(3)     NULL,
  created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_notifications_user (user_id, id),
  KEY idx_notifications_unread (user_id, read_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE device_tokens (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id       BIGINT UNSIGNED NOT NULL,
  platform      ENUM('android','ios','web') NOT NULL,
  token         VARCHAR(512)    NOT NULL,
  app_version   VARCHAR(16)     NULL,
  last_seen_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uq_device_token (token),
  KEY idx_device_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
