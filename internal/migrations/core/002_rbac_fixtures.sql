-- +goose Up

-- Groups: superuser, admin, default
INSERT INTO groups (name, description) VALUES
    ('superuser', 'superuser group users who have all right to do anything'),
    ('admin', 'admin group users who have the most right to do very much things'),
    ('default', 'default group users who have all default right')
ON CONFLICT (name) DO NOTHING;

-- Namespace: default
INSERT INTO namespaces (name, description) VALUES
    ('default', 'default namespace')
ON CONFLICT (name) DO NOTHING;

-- RbacPolicy: superuser wildcard
INSERT INTO rbac_policies (ptype, v0, v1, v2, v3) VALUES
    ('p', 'superuser', '*', '*', 'allow')
ON CONFLICT (ptype, v0, v1, v2) DO NOTHING;

-- RbacPolicy: admin group policies
INSERT INTO rbac_policies (ptype, v0, v1, v2, v3) VALUES
    ('p', 'admin', '/api/v1/admin/user/list', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/user/list', 'OPTIONS', 'allow'),
    ('p', 'admin', '/api/v1/admin/user/:unionID/activate', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/user/:unionID/deactivate', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/user/:unionID/join-group', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/user/:unionID/leave-group', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/group/list', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/group/list', 'OPTIONS', 'allow'),
    ('p', 'admin', '/api/v1/admin/group/:name/get', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/group/:name/add-rbac-policy', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/group/:name/remove-rbac-policy', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/rbac-policy-template/list', 'POST', 'allow'),
    ('p', 'admin', '/api/v1/admin/rbac-policy-template/list', 'OPTIONS', 'allow')
ON CONFLICT (ptype, v0, v1, v2) DO NOTHING;

-- RbacPolicyTemplate
INSERT INTO rbac_policy_templates (target, action) VALUES
    ('/api/v1/admin/user/list', 'POST'),
    ('/api/v1/admin/user/list', 'OPTIONS'),
    ('/api/v1/admin/user/:unionID/get', 'POST'),
    ('/api/v1/admin/user/:unionID/activate', 'POST'),
    ('/api/v1/admin/user/:unionID/deactivate', 'POST'),
    ('/api/v1/admin/user/:unionID/join-group', 'POST'),
    ('/api/v1/admin/user/:unionID/leave-group', 'POST'),
    ('/api/v1/admin/group/list', 'POST'),
    ('/api/v1/admin/group/list', 'OPTIONS'),
    ('/api/v1/admin/group/:name/get', 'POST'),
    ('/api/v1/admin/group/:name/add-rbac-policy', 'POST'),
    ('/api/v1/admin/group/:name/remove-rbac-policy', 'POST'),
    ('/api/v1/admin/rbac-policy-template/list', 'POST'),
    ('/api/v1/admin/rbac-policy-template/list', 'OPTIONS'),
    ('/api/v1/admin/namespace/list', 'POST'),
    ('/api/v1/admin/namespace/list', 'OPTIONS')
ON CONFLICT (target, action) DO NOTHING;

-- +goose Down
DELETE FROM rbac_policy_templates WHERE target LIKE '/api/v1/admin/%';
DELETE FROM rbac_policies WHERE v0 IN ('superuser', 'admin') AND ptype = 'p';
DELETE FROM namespaces WHERE name = 'default';
DELETE FROM groups WHERE name IN ('superuser', 'admin', 'default');
