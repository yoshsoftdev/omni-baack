UPDATE contacts SET phone = 'rollback:' || LEFT(id::text, 23) WHERE phone IS NULL;
ALTER TABLE contacts ALTER COLUMN phone SET NOT NULL;

DROP INDEX IF EXISTS idx_conversations_provider_profile_id;
DROP TRIGGER IF EXISTS conversations_provider_profile_before_write ON conversations;
DROP FUNCTION IF EXISTS ensure_conversation_provider_profile();
ALTER TABLE conversations DROP COLUMN IF EXISTS provider_profile_id;
ALTER TABLE conversations DROP COLUMN IF EXISTS external_thread_id;

DROP TABLE IF EXISTS contact_provider_activities;

ALTER TABLE contacts DROP CONSTRAINT IF EXISTS contacts_source_of_truth_profile_fkey;
ALTER TABLE contacts DROP COLUMN IF EXISTS source_of_truth_profile_id;

DROP TABLE IF EXISTS provider_profiles;
