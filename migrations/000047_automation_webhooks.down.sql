DROP TABLE IF EXISTS automation_webhook_deliveries;

ALTER TABLE automation_endpoints
    DROP COLUMN IF EXISTS webhook_secret,
    DROP COLUMN IF EXISTS webhook_enabled,
    DROP COLUMN IF EXISTS webhook_url;
