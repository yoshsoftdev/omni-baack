-- Instagram contact source + longer external message ids (Instagram mids exceed 128 chars).
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_enum e
        JOIN pg_type t ON t.oid = e.enumtypid
        WHERE t.typname = 'contact_source'
          AND e.enumlabel = 'ig'
    ) THEN
        ALTER TYPE contact_source ADD VALUE 'ig';
    END IF;
END
$$;

ALTER TABLE messages
    ALTER COLUMN wamid TYPE VARCHAR(512),
    ALTER COLUMN reply_to_wamid TYPE VARCHAR(512);
