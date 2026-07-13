-- Split contacts.name into contact_name (inbox display) and customer_name (WhatsApp profile / CRM name).

ALTER TABLE contacts ADD COLUMN contact_name VARCHAR(255);
ALTER TABLE contacts ADD COLUMN customer_name VARCHAR(255);

UPDATE contacts
SET contact_name = name,
    customer_name = name
WHERE deleted_at IS NULL;

ALTER TABLE contacts ALTER COLUMN contact_name SET NOT NULL;
ALTER TABLE contacts ALTER COLUMN customer_name SET NOT NULL;

DROP INDEX IF EXISTS idx_contacts_name_trgm;
ALTER TABLE contacts DROP COLUMN name;

CREATE INDEX idx_contacts_contact_name_trgm ON contacts USING GIN (contact_name gin_trgm_ops) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_customer_name_trgm ON contacts USING GIN (customer_name gin_trgm_ops) WHERE deleted_at IS NULL;
