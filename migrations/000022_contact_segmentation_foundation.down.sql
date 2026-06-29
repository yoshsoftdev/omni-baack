DROP TABLE IF EXISTS contact_field_values;
DROP TABLE IF EXISTS contact_field_definitions;

ALTER TABLE contacts
    DROP COLUMN IF EXISTS company,
    DROP COLUMN IF EXISTS country,
    DROP COLUMN IF EXISTS province,
    DROP COLUMN IF EXISTS job_title;

DROP TYPE IF EXISTS contact_field_type;
