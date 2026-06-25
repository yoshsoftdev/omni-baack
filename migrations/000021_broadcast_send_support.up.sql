-- Phase 12.2: send worker support
ALTER TABLE campaign_recipients
    ADD COLUMN IF NOT EXISTS provider_response JSONB;
