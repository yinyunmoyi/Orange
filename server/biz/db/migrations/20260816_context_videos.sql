USE aaa_word;

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
