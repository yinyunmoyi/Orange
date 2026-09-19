USE aaa_word;

CREATE TABLE IF NOT EXISTS learning_settings (
  id                 TINYINT PRIMARY KEY,
  daily_new_limit    INT NOT NULL,
  daily_review_limit INT NOT NULL,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
