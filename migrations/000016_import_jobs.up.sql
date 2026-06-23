-- Phase 9A: contact import jobs, files, row results (idempotent)
CREATE TABLE IF NOT EXISTS contact_import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    source import_source NOT NULL,
    status import_job_status NOT NULL DEFAULT 'pending',
    filename VARCHAR(512) NOT NULL,
    total_rows INTEGER NOT NULL DEFAULT 0,
    imported_count INTEGER NOT NULL DEFAULT 0,
    updated_count INTEGER NOT NULL DEFAULT 0,
    skipped_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    error_summary TEXT,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contact_import_jobs_team_id ON contact_import_jobs (team_id);
CREATE INDEX IF NOT EXISTS idx_contact_import_jobs_team_created_at ON contact_import_jobs (team_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_contact_import_jobs_status ON contact_import_jobs (status);

CREATE TABLE IF NOT EXISTS contact_import_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES contact_import_jobs (id) ON DELETE CASCADE,
    storage_key VARCHAR(512) NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_contact_import_files_job_id ON contact_import_files (job_id);

CREATE TABLE IF NOT EXISTS contact_import_rows (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES contact_import_jobs (id) ON DELETE CASCADE,
    row_number INTEGER NOT NULL,
    status import_row_status NOT NULL,
    contact_id UUID REFERENCES contacts (id) ON DELETE SET NULL,
    raw_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at),
    CONSTRAINT uq_contact_import_rows_job_row_created UNIQUE (job_id, row_number, created_at)
) PARTITION BY RANGE (created_at);

DO $$
BEGIN
    CREATE TABLE contact_import_rows_default PARTITION OF contact_import_rows DEFAULT;
EXCEPTION
    WHEN duplicate_table THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_contact_import_rows_job_id ON contact_import_rows (job_id);
CREATE INDEX IF NOT EXISTS idx_contact_import_rows_job_status ON contact_import_rows (job_id, status);

DO $$
BEGIN
    ALTER TABLE contacts
        ADD CONSTRAINT fk_contacts_import_job_id
        FOREIGN KEY (import_job_id) REFERENCES contact_import_jobs (id) ON DELETE SET NULL;
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;
