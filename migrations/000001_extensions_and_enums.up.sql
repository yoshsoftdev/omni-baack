-- Phase 9A: extensions and shared enum types
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE plan_type AS ENUM ('starter', 'growth', 'business', 'enterprise');

CREATE TYPE team_role AS ENUM ('owner', 'admin', 'supervisor', 'agent');

CREATE TYPE member_status AS ENUM ('active', 'inactive', 'invited');

CREATE TYPE invitation_status AS ENUM ('pending', 'accepted', 'expired', 'cancelled');

CREATE TYPE channel_status AS ENUM ('disconnected', 'connecting', 'connected');

CREATE TYPE channel_connection_type AS ENUM ('manual', 'embedded_coex');

CREATE TYPE channel_onboarding_status AS ENUM (
    'pending',
    'syncing_contacts',
    'syncing_history',
    'completed',
    'failed'
);

CREATE TYPE template_status AS ENUM ('APPROVED', 'PENDING', 'REJECTED');

CREATE TYPE template_category AS ENUM ('MARKETING', 'UTILITY', 'AUTHENTICATION');

CREATE TYPE campaign_status AS ENUM (
    'draft',
    'scheduled',
    'queued',
    'sending',
    'completed',
    'failed',
    'cancelled'
);

CREATE TYPE recipient_status AS ENUM (
    'pending',
    'queued',
    'sending',
    'sent',
    'delivered',
    'read',
    'failed',
    'cancelled',
    'skipped'
);

CREATE TYPE audience_source_type AS ENUM ('segment', 'tag', 'contact');

CREATE TYPE broadcast_job_status AS ENUM (
    'pending',
    'resolving_audience',
    'planning_batches',
    'sending',
    'completed',
    'failed',
    'cancelled'
);

CREATE TYPE broadcast_batch_status AS ENUM (
    'pending',
    'queued',
    'processing',
    'completed',
    'failed',
    'cancelled'
);

CREATE TYPE marketing_status AS ENUM ('subscribed', 'unsubscribed', 'unknown');

CREATE TYPE contact_source AS ENUM ('whatsapp', 'import', 'manual', 'api');

CREATE TYPE segment_rule_field AS ENUM (
    'tag',
    'city',
    'last_activity',
    'marketing_status',
    'source'
);

CREATE TYPE segment_rule_operator AS ENUM (
    'equals',
    'not_equals',
    'contains',
    'less_than',
    'greater_than'
);

CREATE TYPE conversation_status AS ENUM ('open', 'assigned', 'closed');

-- Named conversation_channel_type to avoid shadowing column name channel_type in conversations.
CREATE TYPE conversation_channel_type AS ENUM ('whatsapp', 'instagram', 'other');

CREATE TYPE message_direction AS ENUM ('inbound', 'outbound');

CREATE TYPE message_status AS ENUM ('sent', 'delivered', 'read', 'failed');

CREATE TYPE message_content_type AS ENUM (
    'text',
    'image',
    'video',
    'audio',
    'document',
    'template',
    'other'
);

CREATE TYPE import_job_status AS ENUM ('pending', 'processing', 'completed', 'failed');

CREATE TYPE import_source AS ENUM ('csv', 'excel', 'manual');

CREATE TYPE import_row_status AS ENUM ('imported', 'updated', 'skipped', 'failed');

CREATE TYPE send_mode AS ENUM ('now', 'later');
