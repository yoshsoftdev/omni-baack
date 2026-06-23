ALTER TABLE contacts DROP CONSTRAINT IF EXISTS fk_contacts_import_job_id;
DROP TRIGGER IF EXISTS trg_campaign_recipients_status_monotonic ON campaign_recipients;
DROP FUNCTION IF EXISTS enforce_recipient_status_monotonic();
DROP TABLE IF EXISTS campaign_recipients;
DROP TABLE IF EXISTS campaign_audience_snapshots;
DROP TABLE IF EXISTS broadcast_batches;
DROP TABLE IF EXISTS broadcast_jobs;
