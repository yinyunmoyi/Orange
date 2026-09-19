USE aaa_word;

ALTER TABLE context_tasks
  CHANGE COLUMN sentence raw_paragraph MEDIUMTEXT NOT NULL,
  CHANGE COLUMN highlight_start raw_selection_start INT NOT NULL,
  CHANGE COLUMN highlight_end raw_selection_end INT NOT NULL,
  ADD COLUMN splitter_version VARCHAR(32) NOT NULL DEFAULT 'legacy_client_v1' AFTER raw_selection_end,
  ADD COLUMN extracted_sentence TEXT NULL AFTER splitter_version,
  ADD COLUMN extracted_sentence_start INT NULL AFTER extracted_sentence,
  ADD COLUMN extracted_sentence_end INT NULL AFTER extracted_sentence_start,
  ADD COLUMN extracted_highlight_start INT NULL AFTER extracted_sentence_end,
  ADD COLUMN extracted_highlight_end INT NULL AFTER extracted_highlight_start;

UPDATE context_tasks
SET extracted_sentence = raw_paragraph,
    extracted_sentence_start = 0,
    extracted_sentence_end = CHAR_LENGTH(raw_paragraph),
    extracted_highlight_start = raw_selection_start,
    extracted_highlight_end = raw_selection_end
WHERE extracted_sentence IS NULL;
