DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE code IN (
        'automation.view',
        'automation.create',
        'automation.update',
        'automation.delete',
        'automation.invoke'
    )
);

DELETE FROM permissions
WHERE code IN (
    'automation.view',
    'automation.create',
    'automation.update',
    'automation.delete',
    'automation.invoke'
);

DROP TABLE IF EXISTS automation_executions;
DROP TABLE IF EXISTS automation_api_keys;
DROP TABLE IF EXISTS automation_endpoints;
