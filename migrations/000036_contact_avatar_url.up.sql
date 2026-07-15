-- Store Instagram (and other channel) profile pictures on contacts.
ALTER TABLE contacts
    ADD COLUMN IF NOT EXISTS avatar_url TEXT;
