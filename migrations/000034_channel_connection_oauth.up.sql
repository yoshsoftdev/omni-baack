-- Allow Instagram Business Login (and other OAuth connections) on channels.connection_type.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_enum e
        JOIN pg_type t ON t.oid = e.enumtypid
        WHERE t.typname = 'channel_connection_type'
          AND e.enumlabel = 'oauth'
    ) THEN
        ALTER TYPE channel_connection_type ADD VALUE 'oauth';
    END IF;
END
$$;
