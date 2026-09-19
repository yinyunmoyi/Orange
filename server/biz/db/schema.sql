-- 单词收藏功能数据库初始化脚本
-- 手动执行：mysql -u root -p < biz/db/schema.sql

CREATE DATABASE IF NOT EXISTS aaa_word DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE aaa_word;

-- 收藏词主表：一词一行
CREATE TABLE IF NOT EXISTS words (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  word       VARCHAR(64) NOT NULL,
  level      INT NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_word (word),
  KEY idx_words_created_at (created_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 词性 + 释义（中英统一用 kind 字段区分）
CREATE TABLE IF NOT EXISTS word_meanings (
  id             BIGINT AUTO_INCREMENT PRIMARY KEY,
  word_id        BIGINT NOT NULL,
  kind           ENUM('zh','en') NOT NULL,
  part_of_speech VARCHAR(32) NOT NULL DEFAULT '',
  definition     TEXT NOT NULL,
  example        TEXT NOT NULL,
  synonyms       JSON NULL,
  antonyms       JSON NULL,
  sort_order     INT NOT NULL DEFAULT 0,
  KEY idx_word_id_kind (word_id, kind, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 发音及音频元数据
CREATE TABLE IF NOT EXISTS word_audios (
  id            BIGINT AUTO_INCREMENT PRIMARY KEY,
  word_id       BIGINT NOT NULL,
  accent        VARCHAR(8) NOT NULL DEFAULT 'EN',
  phonetic_text VARCHAR(128) NOT NULL DEFAULT '',
  source_url    VARCHAR(512) NOT NULL,
  file_path     VARCHAR(255) NOT NULL,
  content_type  VARCHAR(64) NOT NULL DEFAULT 'audio/mpeg',
  file_size     BIGINT NOT NULL DEFAULT 0,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_word_accent (word_id, accent),
  UNIQUE KEY uk_source_url (source_url),
  KEY idx_word_id (word_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 收藏短语主表：短语只有一个美音，不保存音标
CREATE TABLE IF NOT EXISTS phrases (
  id                 BIGINT AUTO_INCREMENT PRIMARY KEY,
  phrase             VARCHAR(80) NOT NULL,
  audio_file_path    VARCHAR(255) NOT NULL,
  audio_content_type VARCHAR(64) NOT NULL DEFAULT 'audio/mpeg',
  audio_file_size    BIGINT NOT NULL DEFAULT 0,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_phrase (phrase),
  KEY idx_phrases_created_at (created_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 短语中文释义
CREATE TABLE IF NOT EXISTS phrase_meanings (
  id             BIGINT AUTO_INCREMENT PRIMARY KEY,
  phrase_id      BIGINT NOT NULL,
  part_of_speech VARCHAR(32) NOT NULL DEFAULT '',
  definition     TEXT NOT NULL,
  sort_order     INT NOT NULL DEFAULT 0,
  KEY idx_phrase_meaning (phrase_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 收藏句子：独立于单词/短语学习计划
CREATE TABLE IF NOT EXISTS sentence_favorites (
  id          BIGINT AUTO_INCREMENT PRIMARY KEY,
  sentence    TEXT NOT NULL,
  translation TEXT NOT NULL,
  note        VARCHAR(512) NOT NULL DEFAULT '',
  dedupe_key  CHAR(64) NOT NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_sentence_favorite_dedupe (dedupe_key),
  KEY idx_sentence_favorites_created_at (created_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sentence_tags (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  name       VARCHAR(64) NOT NULL,
  color      CHAR(7) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_sentence_tag_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sentence_favorite_tags (
  sentence_favorite_id BIGINT NOT NULL,
  tag_id               BIGINT NOT NULL,
  created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_sentence_favorite_tag (sentence_favorite_id, tag_id),
  KEY idx_sentence_favorite_tags_tag (tag_id, sentence_favorite_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 单词/短语独立备注：按类型和规范化文本关联，不依赖收藏记录
CREATE TABLE IF NOT EXISTS item_notes (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  item_type  VARCHAR(16) NOT NULL,
  item_text  VARCHAR(80) NOT NULL,
  note       TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_item_note (item_type, item_text)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 通用语境：通过 item_type + item_id 关联已收藏的单词，未来可扩展短语和句子
CREATE TABLE IF NOT EXISTS contexts (
  id                 BIGINT AUTO_INCREMENT PRIMARY KEY,
  item_type          VARCHAR(16) NOT NULL,
  item_id            BIGINT NOT NULL,
  sentence           TEXT NOT NULL,
  translation        TEXT NOT NULL,
  highlight_start    INT NOT NULL,
  highlight_end      INT NOT NULL,
  audio_file_path    VARCHAR(255) NOT NULL,
  audio_content_type VARCHAR(64) NOT NULL DEFAULT 'audio/mpeg',
  audio_file_size    BIGINT NOT NULL DEFAULT 0,
  dedupe_key         CHAR(64) NOT NULL,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_context_dedupe (dedupe_key),
  KEY idx_context_item (item_type, item_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 视频语境：同一文本语境可关联同一视频或不同视频中的多次出现
CREATE TABLE IF NOT EXISTS context_videos (
  id                 BIGINT AUTO_INCREMENT PRIMARY KEY,
  context_id         BIGINT NOT NULL,
  source_fingerprint CHAR(64) NOT NULL,
  clip_start_ms      BIGINT NOT NULL,
  clip_end_ms        BIGINT NOT NULL,
  duration_ms        BIGINT NOT NULL,
  file_path          VARCHAR(255) NOT NULL,
  content_type       VARCHAR(64) NOT NULL DEFAULT 'video/webm',
  file_size          BIGINT NOT NULL DEFAULT 0,
  subtitles_json     MEDIUMTEXT NOT NULL,
  dedupe_key         CHAR(64) NOT NULL,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_context_video_dedupe (dedupe_key),
  KEY idx_context_video_context (context_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 语境同步处理任务：先保存原始输入，再执行翻译、TTS 和语境落库
CREATE TABLE IF NOT EXISTS context_tasks (
  id                BIGINT AUTO_INCREMENT PRIMARY KEY,
  dedupe_key        CHAR(64) NOT NULL,
  item_type         VARCHAR(16) NOT NULL,
  item_id           BIGINT NOT NULL,
  raw_paragraph     MEDIUMTEXT NOT NULL,
  raw_selection_start INT NOT NULL,
  raw_selection_end INT NOT NULL,
  splitter_version  VARCHAR(32) NOT NULL,
  extracted_sentence TEXT NULL,
  extracted_sentence_start INT NULL,
  extracted_sentence_end INT NULL,
  extracted_highlight_start INT NULL,
  extracted_highlight_end INT NULL,
  status            VARCHAR(16) NOT NULL,
  attempts          INT NOT NULL DEFAULT 0,
  last_error        TEXT NOT NULL,
  result_context_id BIGINT NULL,
  started_at        DATETIME NULL,
  finished_at       DATETIME NULL,
  created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_context_task_dedupe (dedupe_key),
  KEY idx_context_task_item (item_type, item_id, created_at),
  KEY idx_context_task_status (status, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_item_learning (
  id                  BIGINT AUTO_INCREMENT PRIMARY KEY,
  item_type           TINYINT NOT NULL,
  item_id             BIGINT NOT NULL,
  queued_at           DATETIME(6) NOT NULL,
  learning_status     VARCHAR(20) NOT NULL,
  memory_stage        INT NOT NULL DEFAULT 0,
  review_success_days INT NOT NULL DEFAULT 0,
  last_reviewed_at    DATETIME NULL,
  next_review_at      DATETIME NULL,
  schedule_version    INT NOT NULL DEFAULT 1,
  learned_at          DATETIME NULL,
  mastered_at         DATETIME NULL,
  created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_learning_item (item_type, item_id),
  KEY idx_learning_due (learning_status, next_review_at),
  KEY idx_learning_queued (queued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS learning_sessions (
  id                    BIGINT AUTO_INCREMENT PRIMARY KEY,
  study_date            DATE NOT NULL,
  status                VARCHAR(20) NOT NULL,
  turn_no               INT NOT NULL DEFAULT 0,
  current_queue_item_id BIGINT NULL,
  created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at          DATETIME NULL,
  updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_learning_session_date (study_date, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_learning_queue (
  id               BIGINT AUTO_INCREMENT PRIMARY KEY,
  session_id       BIGINT NOT NULL,
  item_type        TINYINT NOT NULL,
  item_id          BIGINT NOT NULL,
  queue_type       VARCHAR(20) NOT NULL,
  source_queue_type VARCHAR(20) NOT NULL,
  queue_seq        DECIMAL(10,2) NOT NULL,
  status           VARCHAR(20) NOT NULL,
  had_incorrect    BOOLEAN NOT NULL DEFAULT FALSE,
  incorrect_count  INT NOT NULL DEFAULT 0,
  answer_count     INT NOT NULL DEFAULT 0,
  known_count      INT NOT NULL DEFAULT 0,
  retry_after_turn INT NULL,
  created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at     DATETIME NULL,
  updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_session_item (session_id, item_type, item_id),
  KEY idx_queue_pick (session_id, status, retry_after_turn, queue_seq)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS learning_settings (
  id                 TINYINT PRIMARY KEY,
  daily_new_limit    INT NOT NULL,
  daily_review_limit INT NOT NULL,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS item_action_events (
  id            BIGINT AUTO_INCREMENT PRIMARY KEY,
  event_id      VARCHAR(64) NOT NULL,
  item_type     VARCHAR(16) NOT NULL,
  item_id       BIGINT NOT NULL,
  item_text     VARCHAR(80) NOT NULL,
  action        VARCHAR(32) NOT NULL,
  source        VARCHAR(32) NOT NULL,
  level_before  INT NULL,
  level_after   INT NULL,
  session_id    BIGINT NULL,
  queue_item_id BIGINT NULL,
  context_id    BIGINT NULL,
  metadata      JSON NULL,
  occurred_at   DATETIME(6) NOT NULL,
  created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uk_item_action_event_id (event_id),
  KEY idx_item_action_target (item_type, item_id, occurred_at),
  KEY idx_item_action_action (action, occurred_at),
  KEY idx_item_action_source (source, occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS standalone_sync_receipts (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  client_id    VARCHAR(64) NOT NULL,
  payload_hash CHAR(64) NOT NULL,
  item_type    VARCHAR(16) NOT NULL,
  item_id      BIGINT NOT NULL,
  created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uk_standalone_sync_client (client_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 相似单词组
CREATE TABLE IF NOT EXISTS word_groups (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  member_count INT NOT NULL DEFAULT 0,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 相似单词组与单词的关系（一个词只能属于一个组）
CREATE TABLE IF NOT EXISTS word_group_members (
  id       BIGINT AUTO_INCREMENT PRIMARY KEY,
  group_id BIGINT NOT NULL,
  word_id  BIGINT NOT NULL,
  added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_word_id (word_id),
  KEY idx_group_id (group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
