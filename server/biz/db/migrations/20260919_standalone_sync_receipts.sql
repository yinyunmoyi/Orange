USE aaa_word;

CREATE TABLE IF NOT EXISTS standalone_sync_receipts (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  client_id    VARCHAR(64) NOT NULL,
  payload_hash CHAR(64) NOT NULL,
  item_type    VARCHAR(16) NOT NULL,
  item_id      BIGINT NOT NULL,
  created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uk_standalone_sync_client (client_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
