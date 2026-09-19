USE aaa_word;

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
  note                 VARCHAR(512) NOT NULL,
  created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_sentence_favorite_tag (sentence_favorite_id, tag_id),
  KEY idx_sentence_favorite_tags_tag (tag_id, sentence_favorite_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
