USE aaa_word;

ALTER TABLE words
  ADD INDEX idx_words_created_at (created_at, id);

ALTER TABLE phrases
  ADD INDEX idx_phrases_created_at (created_at, id);
