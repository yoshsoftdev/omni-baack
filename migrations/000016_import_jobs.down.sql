ALTER TABLE contacts DROP CONSTRAINT IF EXISTS fk_contacts_import_job_id;
DROP TABLE IF EXISTS contact_import_rows;
DROP TABLE IF EXISTS contact_import_files;
DROP TABLE IF EXISTS contact_import_jobs;
