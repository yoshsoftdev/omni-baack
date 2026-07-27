-- Sprint 3 Phase 5: repair invalid locale artifacts and refresh normalized_language.
-- Only rewrite known-bad en_EN (invented by legacy normalizeMetaLanguage).
-- Do not rewrite legitimate Meta short codes that may coexist under unique (name, language).

UPDATE templates
SET
    language = 'en_US',
    normalized_language = 'en_US',
    updated_at = NOW()
WHERE deleted_at IS NULL
  AND lower(language) = 'en_en'
  AND NOT EXISTS (
      SELECT 1
      FROM templates t2
      WHERE t2.deleted_at IS NULL
        AND t2.channel_id = templates.channel_id
        AND t2.name = templates.name
        AND t2.language = 'en_US'
        AND t2.id <> templates.id
  );

UPDATE templates
SET normalized_language = CASE
    WHEN lower(language) IN ('en', 'en_us', 'en_en') THEN 'en_US'
    WHEN lower(language) IN ('id', 'id_id') THEN 'id'
    ELSE language
END
WHERE deleted_at IS NULL;
