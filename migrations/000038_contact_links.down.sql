DROP TABLE IF EXISTS contact_links;

ALTER TABLE contacts
    DROP CONSTRAINT IF EXISTS contacts_id_team_unique;
