USE aaa_word;

ALTER TABLE user_item_learning
  MODIFY COLUMN queued_at DATETIME(6) NOT NULL;
