-- Canonical provider profiles and immutable conversation ownership.
CREATE TABLE provider_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    contact_id UUID NOT NULL REFERENCES contacts (id) ON DELETE RESTRICT,
    provider VARCHAR(64) NOT NULL,
    provider_user_id VARCHAR(512) NOT NULL,
    channel_id UUID REFERENCES channels (id) ON DELETE RESTRICT,
    display_name VARCHAR(255),
    username VARCHAR(255),
    phone VARCHAR(32),
    avatar_url TEXT,
    email VARCHAR(255),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    connected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_synced_at TIMESTAMPTZ,
    last_observed_at TIMESTAMPTZ,
    status VARCHAR(32) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'unavailable', 'unlinked')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

ALTER TABLE provider_profiles
    ADD CONSTRAINT provider_profiles_owner_identity_unique UNIQUE (id, contact_id, team_id);

CREATE UNIQUE INDEX idx_provider_profiles_identity_active
    ON provider_profiles (
        team_id,
        provider,
        COALESCE(channel_id, '00000000-0000-0000-0000-000000000000'::uuid),
        provider_user_id
    )
    WHERE deleted_at IS NULL;
CREATE INDEX idx_provider_profiles_contact ON provider_profiles (team_id, contact_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_provider_profiles_search ON provider_profiles
    USING GIN ((COALESCE(display_name, '') || ' ' || COALESCE(username, '') || ' ' ||
                COALESCE(phone, '') || ' ' || COALESCE(email, '')) gin_trgm_ops)
    WHERE deleted_at IS NULL;

ALTER TABLE contacts
    ADD COLUMN source_of_truth_profile_id UUID;

-- Existing Instagram provisional keys are migrated as Instagram profiles.
INSERT INTO provider_profiles (
    team_id, contact_id, provider, provider_user_id, display_name, username,
    avatar_url, connected_at, created_at, updated_at
)
SELECT
    c.team_id,
    c.id,
    'instagram',
    regexp_replace(c.phone, '^ig:', ''),
    COALESCE(NULLIF(c.customer_name, ''), c.contact_name),
    CASE WHEN c.contact_name LIKE '@%' THEN regexp_replace(c.contact_name, '^@', '') END,
    c.avatar_url,
    c.created_at,
    c.created_at,
    c.updated_at
FROM contacts c
WHERE c.deleted_at IS NULL
  AND (c.source::text = 'ig' OR c.phone LIKE 'ig:%')
  AND NULLIF(regexp_replace(c.phone, '^ig:', ''), '') IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM conversations cv
      WHERE cv.team_id = c.team_id AND cv.contact_id = c.id AND cv.deleted_at IS NULL
  )
ON CONFLICT DO NOTHING;

-- Existing phone-based contacts remain fully backward compatible as WhatsApp profiles.
INSERT INTO provider_profiles (
    team_id, contact_id, provider, provider_user_id, display_name, phone,
    avatar_url, email, connected_at, created_at, updated_at
)
SELECT
    c.team_id,
    c.id,
    'whatsapp',
    c.phone,
    COALESCE(NULLIF(c.customer_name, ''), c.contact_name),
    c.phone,
    c.avatar_url,
    c.email,
    c.created_at,
    c.created_at,
    c.updated_at
FROM contacts c
WHERE c.deleted_at IS NULL
  AND NULLIF(c.phone, '') IS NOT NULL
  AND c.phone NOT LIKE 'ig:%'
  AND c.source::text IN ('whatsapp', 'meta_inbound', 'meta_sync')
  AND NOT EXISTS (
      SELECT 1 FROM conversations cv
      WHERE cv.team_id = c.team_id AND cv.contact_id = c.id AND cv.deleted_at IS NULL
  )
ON CONFLICT DO NOTHING;

UPDATE contacts c
SET source_of_truth_profile_id = p.id
FROM provider_profiles p
WHERE p.contact_id = c.id
  AND p.team_id = c.team_id
  AND p.provider = 'whatsapp'
  AND p.deleted_at IS NULL
  AND c.source_of_truth_profile_id IS NULL;

ALTER TABLE contacts
    ADD CONSTRAINT contacts_source_of_truth_profile_fkey
    FOREIGN KEY (source_of_truth_profile_id, id, team_id)
    REFERENCES provider_profiles (id, contact_id, team_id)
    DEFERRABLE INITIALLY IMMEDIATE;

CREATE TABLE contact_provider_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    contact_id UUID NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    provider_profile_id UUID REFERENCES provider_profiles (id) ON DELETE SET NULL,
    previous_contact_id UUID REFERENCES contacts (id) ON DELETE SET NULL,
    new_contact_id UUID REFERENCES contacts (id) ON DELETE SET NULL,
    activity_type VARCHAR(64) NOT NULL CHECK (
        activity_type IN ('provider_linked', 'provider_unlinked', 'source_of_truth_changed')
    ),
    actor_id UUID REFERENCES users (id) ON DELETE SET NULL,
    description TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_contact_provider_activities_contact
    ON contact_provider_activities (team_id, contact_id, created_at DESC);

ALTER TABLE conversations
    ADD COLUMN provider_profile_id UUID REFERENCES provider_profiles (id) ON DELETE RESTRICT,
    ADD COLUMN external_thread_id VARCHAR(512);

-- Preserve every conversation's original provider/channel identity.
INSERT INTO provider_profiles (
    team_id, contact_id, provider, provider_user_id, channel_id, display_name,
    phone, avatar_url, connected_at, created_at, updated_at
)
SELECT DISTINCT ON (cv.team_id, cv.contact_id, cv.channel_id, cv.channel_type)
    cv.team_id,
    cv.contact_id,
    CASE WHEN cv.channel_type::text = 'instagram' THEN 'instagram' ELSE 'whatsapp' END,
    CASE
        WHEN cv.channel_type::text = 'instagram' AND c.phone LIKE 'ig:%' THEN regexp_replace(c.phone, '^ig:', '')
        ELSE c.phone
    END,
    cv.channel_id,
    COALESCE(NULLIF(c.customer_name, ''), c.contact_name),
    CASE WHEN cv.channel_type::text = 'instagram' THEN NULL ELSE c.phone END,
    c.avatar_url,
    cv.created_at,
    cv.created_at,
    cv.updated_at
FROM conversations cv
JOIN contacts c ON c.id = cv.contact_id AND c.team_id = cv.team_id
WHERE cv.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM provider_profiles p
      WHERE p.team_id = cv.team_id
        AND p.contact_id = cv.contact_id
        AND p.provider = CASE WHEN cv.channel_type::text = 'instagram' THEN 'instagram' ELSE 'whatsapp' END
        AND p.channel_id IS NOT DISTINCT FROM cv.channel_id
        AND p.deleted_at IS NULL
  )
  AND NULLIF(c.phone, '') IS NOT NULL
ON CONFLICT DO NOTHING;

UPDATE conversations cv
SET provider_profile_id = p.id
FROM provider_profiles p
WHERE p.team_id = cv.team_id
  AND p.contact_id = cv.contact_id
  AND p.provider = CASE WHEN cv.channel_type::text = 'instagram' THEN 'instagram' ELSE 'whatsapp' END
  AND p.channel_id IS NOT DISTINCT FROM cv.channel_id
  AND p.deleted_at IS NULL
  AND cv.provider_profile_id IS NULL;

UPDATE contacts c
SET source_of_truth_profile_id = p.id
FROM provider_profiles p
WHERE p.contact_id = c.id
  AND p.team_id = c.team_id
  AND p.provider = 'whatsapp'
  AND p.deleted_at IS NULL
  AND c.source_of_truth_profile_id IS NULL;

-- Fall back to an unscoped migrated profile when a channel-scoped profile was not required.
UPDATE conversations cv
SET provider_profile_id = p.id
FROM provider_profiles p
WHERE p.team_id = cv.team_id
  AND p.contact_id = cv.contact_id
  AND p.provider = CASE WHEN cv.channel_type::text = 'instagram' THEN 'instagram' ELSE 'whatsapp' END
  AND p.channel_id IS NULL
  AND p.deleted_at IS NULL
  AND cv.provider_profile_id IS NULL;

UPDATE conversations
SET external_thread_id = 'legacy:' || id::text
WHERE external_thread_id IS NULL;

CREATE OR REPLACE FUNCTION ensure_conversation_provider_profile()
RETURNS TRIGGER AS $$
DECLARE
    resolved_provider VARCHAR(32);
    resolved_external_id VARCHAR(512);
    resolved_profile_id UUID;
    resolved_contact_id UUID;
BEGIN
    IF TG_OP = 'UPDATE'
       AND OLD.provider_profile_id IS NOT NULL
       AND NEW.provider_profile_id IS DISTINCT FROM OLD.provider_profile_id THEN
        RAISE EXCEPTION 'conversation provider profile is immutable';
    END IF;
    IF TG_OP = 'UPDATE' AND NEW.contact_id IS DISTINCT FROM OLD.contact_id THEN
        RAISE EXCEPTION 'conversation legacy contact identity is immutable';
    END IF;
    IF TG_OP = 'UPDATE'
       AND OLD.external_thread_id IS NOT NULL
       AND NEW.external_thread_id IS DISTINCT FROM OLD.external_thread_id THEN
        RAISE EXCEPTION 'conversation external thread identity is immutable';
    END IF;

    NEW.external_thread_id := COALESCE(NULLIF(NEW.external_thread_id, ''), 'legacy:' || NEW.id::text);

    IF NEW.provider_profile_id IS NOT NULL THEN
        SELECT contact_id INTO resolved_contact_id
        FROM provider_profiles
        WHERE id = NEW.provider_profile_id AND team_id = NEW.team_id AND deleted_at IS NULL;
        IF resolved_contact_id IS NULL THEN
            RAISE EXCEPTION 'provider profile does not belong to conversation team';
        END IF;
        NEW.contact_id := resolved_contact_id;
        RETURN NEW;
    END IF;

    resolved_provider := CASE WHEN NEW.channel_type::text = 'instagram' THEN 'instagram' ELSE 'whatsapp' END;

    SELECT id INTO resolved_profile_id
    FROM provider_profiles
    WHERE team_id = NEW.team_id
      AND contact_id = NEW.contact_id
      AND provider = resolved_provider
      AND (channel_id IS NOT DISTINCT FROM NEW.channel_id OR channel_id IS NULL)
      AND deleted_at IS NULL
    ORDER BY (channel_id IS NOT DISTINCT FROM NEW.channel_id) DESC, connected_at ASC
    LIMIT 1;

    IF resolved_profile_id IS NOT NULL THEN
        UPDATE provider_profiles
        SET channel_id = COALESCE(channel_id, NEW.channel_id), updated_at = NOW()
        WHERE id = resolved_profile_id;
        NEW.provider_profile_id := resolved_profile_id;
        RETURN NEW;
    END IF;

    SELECT
        CASE
            WHEN resolved_provider = 'instagram' AND c.phone LIKE 'ig:%' THEN regexp_replace(c.phone, '^ig:', '')
            ELSE c.phone
        END
    INTO resolved_external_id
    FROM contacts c
    WHERE c.id = NEW.contact_id AND c.team_id = NEW.team_id AND c.deleted_at IS NULL;

    IF NULLIF(resolved_external_id, '') IS NULL THEN
        resolved_external_id := NEW.contact_id::text;
    END IF;

    SELECT id INTO resolved_profile_id
    FROM provider_profiles
    WHERE team_id = NEW.team_id
      AND provider = resolved_provider
      AND provider_user_id = resolved_external_id
      AND channel_id IS NOT DISTINCT FROM NEW.channel_id
      AND deleted_at IS NULL
    LIMIT 1;

    IF resolved_profile_id IS NULL THEN
        INSERT INTO provider_profiles (
            team_id, contact_id, provider, provider_user_id, channel_id,
            display_name, phone, avatar_url
        )
        SELECT
            NEW.team_id,
            c.id,
            resolved_provider,
            resolved_external_id,
            NEW.channel_id,
            COALESCE(NULLIF(c.customer_name, ''), c.contact_name),
            CASE WHEN resolved_provider = 'whatsapp' THEN c.phone ELSE NULL END,
            c.avatar_url
        FROM contacts c
        WHERE c.id = NEW.contact_id AND c.team_id = NEW.team_id
        RETURNING id INTO resolved_profile_id;
    END IF;

    NEW.provider_profile_id := resolved_profile_id;
    IF resolved_provider = 'whatsapp' THEN
        UPDATE contacts
        SET source_of_truth_profile_id = COALESCE(source_of_truth_profile_id, resolved_profile_id)
        WHERE id = NEW.contact_id AND team_id = NEW.team_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER conversations_provider_profile_before_write
BEFORE INSERT OR UPDATE OF provider_profile_id, contact_id, external_thread_id ON conversations
FOR EACH ROW EXECUTE FUNCTION ensure_conversation_provider_profile();

ALTER TABLE conversations
    ALTER COLUMN provider_profile_id SET NOT NULL,
    ALTER COLUMN external_thread_id SET NOT NULL;

CREATE INDEX idx_conversations_provider_profile_id
    ON conversations (provider_profile_id) WHERE deleted_at IS NULL;

-- A Contact may be provisional when it has no WhatsApp source of truth.
ALTER TABLE contacts ALTER COLUMN phone DROP NOT NULL;

-- Remove the legacy Instagram identity shim after profiles and conversations are backfilled.
UPDATE contacts
SET phone = NULL, avatar_url = NULL, updated_at = NOW()
WHERE source::text = 'ig' OR phone LIKE 'ig:%';
