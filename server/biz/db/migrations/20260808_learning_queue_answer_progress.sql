USE aaa_word;

ALTER TABLE user_learning_queue
  ADD COLUMN answer_count INT NOT NULL DEFAULT 0 AFTER incorrect_count,
  ADD COLUMN known_count INT NOT NULL DEFAULT 0 AFTER answer_count;
