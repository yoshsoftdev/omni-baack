ALTER TABLE IF EXISTS automation_forward_webhooks
    ADD COLUMN IF NOT EXISTS assign_automation_on_reopen BOOLEAN NOT NULL DEFAULT FALSE;
