ALTER TABLE contacts ADD COLUMN name VARCHAR(255);

UPDATE contacts
SET name = contact_name
WHERE deleted_at IS NULL;

ALTER TABLE contacts ALTER COLUMN name SET NOT NULL;

DROP INDEX IF EXISTS idx_contacts_contact_name_trgm;
DROP INDEX IF EXISTS idx_contacts_customer_name_trgm;

ALTER TABLE contacts DROP COLUMN contact_name;
ALTER TABLE contacts DROP COLUMN customer_name;

CREATE INDEX idx_contacts_name_trgm ON contacts USING GIN (name gin_trgm_ops) WHERE deleted_at IS NULL;
