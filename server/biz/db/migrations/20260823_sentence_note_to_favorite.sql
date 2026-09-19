USE aaa_word;

ALTER TABLE sentence_favorites
  ADD COLUMN note VARCHAR(512) NOT NULL DEFAULT '' AFTER translation;

UPDATE sentence_favorites AS favorites
LEFT JOIN (
  SELECT
    sentence_favorite_id,
    LEFT(
      GROUP_CONCAT(
        NULLIF(TRIM(note), '')
        ORDER BY created_at, tag_id
        SEPARATOR '；'
      ),
      300
    ) AS merged_note
  FROM sentence_favorite_tags
  GROUP BY sentence_favorite_id
) AS relation_notes
  ON relation_notes.sentence_favorite_id = favorites.id
SET favorites.note = COALESCE(relation_notes.merged_note, '')
WHERE favorites.note = '';

ALTER TABLE sentence_favorite_tags
  DROP COLUMN note;
