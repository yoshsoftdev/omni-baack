DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE code IN ('quickreply.view', 'quickreply.create', 'quickreply.update', 'quickreply.delete')
);

DELETE FROM permissions
WHERE code IN ('quickreply.view', 'quickreply.create', 'quickreply.update', 'quickreply.delete');

DROP TABLE IF EXISTS quick_replies;
