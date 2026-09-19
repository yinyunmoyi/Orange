USE aaa_word;

ALTER TABLE words
  ADD COLUMN level INT NOT NULL DEFAULT 1 AFTER word;

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
