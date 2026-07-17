-- Bidirectional relationships between independent CRM contacts.
ALTER TABLE contacts
    ADD CONSTRAINT contacts_id_team_unique UNIQUE (id, team_id);

CREATE TABLE contact_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL,
    contact_a_id UUID NOT NULL,
    contact_b_id UUID NOT NULL,
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT contact_links_distinct_contacts CHECK (contact_a_id < contact_b_id),
    CONSTRAINT contact_links_contact_a_team_fkey
        FOREIGN KEY (contact_a_id, team_id) REFERENCES contacts (id, team_id) ON DELETE RESTRICT,
    CONSTRAINT contact_links_contact_b_team_fkey
        FOREIGN KEY (contact_b_id, team_id) REFERENCES contacts (id, team_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX idx_contact_links_active_pair
    ON contact_links (team_id, contact_a_id, contact_b_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_contact_links_contact_a
    ON contact_links (team_id, contact_a_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_contact_links_contact_b
    ON contact_links (team_id, contact_b_id)
    WHERE deleted_at IS NULL;
