-- Backfill Instagram scoped recipient keys for canonical provider contacts.
UPDATE contacts c
SET phone = 'ig:' || p.provider_user_id,
    source = 'ig',
    updated_at = NOW()
FROM provider_profiles p
WHERE p.contact_id = c.id
  AND p.team_id = c.team_id
  AND p.provider = 'instagram'
  AND p.deleted_at IS NULL
  AND c.deleted_at IS NULL
  AND COALESCE(p.provider_user_id, '') <> ''
  AND (COALESCE(c.phone, '') = '' OR c.phone NOT LIKE 'ig:%');
