USE aaa_word;

CREATE TABLE IF NOT EXISTS user_item_learning (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  item_type TINYINT NOT NULL,
  item_id BIGINT NOT NULL,
  queued_at DATETIME NOT NULL,
  learning_status VARCHAR(20) NOT NULL,
  memory_stage INT NOT NULL DEFAULT 0,
  review_success_days INT NOT NULL DEFAULT 0,
  last_reviewed_at DATETIME NULL,
  next_review_at DATETIME NULL,
  schedule_version INT NOT NULL DEFAULT 1,
  learned_at DATETIME NULL,
  mastered_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_learning_item (item_type, item_id),
  KEY idx_learning_due (learning_status, next_review_at),
  KEY idx_learning_queued (queued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS learning_sessions (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  study_date DATE NOT NULL,
  status VARCHAR(20) NOT NULL,
  turn_no INT NOT NULL DEFAULT 0,
  current_queue_item_id BIGINT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at DATETIME NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_learning_session_date (study_date, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_learning_queue (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  session_id BIGINT NOT NULL,
  item_type TINYINT NOT NULL,
  item_id BIGINT NOT NULL,
  queue_type VARCHAR(20) NOT NULL,
  source_queue_type VARCHAR(20) NOT NULL,
  queue_seq DECIMAL(10,2) NOT NULL,
  status VARCHAR(20) NOT NULL,
  had_incorrect BOOLEAN NOT NULL DEFAULT FALSE,
  incorrect_count INT NOT NULL DEFAULT 0,
  answer_count INT NOT NULL DEFAULT 0,
  known_count INT NOT NULL DEFAULT 0,
  retry_after_turn INT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at DATETIME NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_session_item (session_id, item_type, item_id),
  KEY idx_queue_pick (session_id, status, retry_after_turn, queue_seq)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
