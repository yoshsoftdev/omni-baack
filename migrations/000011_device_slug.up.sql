ALTER TABLE devices ADD COLUMN slug VARCHAR(64);

CREATE UNIQUE INDEX idx_devices_slug ON devices (slug) WHERE deleted_at IS NULL AND slug IS NOT NULL;
