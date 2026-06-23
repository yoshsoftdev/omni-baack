# Legacy migrations (pre–Omni-Chan v1)

These files were removed from the active migrate path to avoid version conflicts with Phase 9A v1 migrations (`000001`–`000016`).

**Do not delete** — reference for upgrading existing databases that used:

- `devices` (not `channels`)
- `campaign_contacts` (not `campaign_recipients`)
- `broadcasts` / `broadcast_recipients` (removed in 000008)

For fresh installs, use only the parent `migrations/` folder.
