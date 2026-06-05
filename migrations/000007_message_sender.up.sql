ALTER TABLE messages
    ADD COLUMN sent_by UUID REFERENCES users (id);

CREATE INDEX idx_messages_sent_by ON messages (sent_by)
    WHERE sent_by IS NOT NULL AND deleted_at IS NULL;
