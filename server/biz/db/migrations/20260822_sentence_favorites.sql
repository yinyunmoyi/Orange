USE aaa_word;

CREATE TABLE IF NOT EXISTS sentence_favorites (
  id          BIGINT AUTO_INCREMENT PRIMARY KEY,
  sentence    TEXT NOT NULL,
  translation TEXT NOT NULL,
  dedupe_key  CHAR(64) NOT NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_sentence_favorite_dedupe (dedupe_key),
  KEY idx_sentence_favorites_created_at (created_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
