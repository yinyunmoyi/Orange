USE aaa_word;

CREATE TABLE IF NOT EXISTS phrases (
  id                 BIGINT AUTO_INCREMENT PRIMARY KEY,
  phrase             VARCHAR(80) NOT NULL,
  audio_file_path    VARCHAR(255) NOT NULL,
  audio_content_type VARCHAR(64) NOT NULL DEFAULT 'audio/mpeg',
  audio_file_size    BIGINT NOT NULL DEFAULT 0,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_phrase (phrase)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS phrase_meanings (
  id             BIGINT AUTO_INCREMENT PRIMARY KEY,
  phrase_id      BIGINT NOT NULL,
  part_of_speech VARCHAR(32) NOT NULL DEFAULT '',
  definition     TEXT NOT NULL,
  sort_order     INT NOT NULL DEFAULT 0,
  KEY idx_phrase_meaning (phrase_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
