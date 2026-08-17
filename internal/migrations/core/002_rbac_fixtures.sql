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

-- TagRuleSample: 标签规则验证样例数据（第二层运行期合法性校验用）
CREATE TABLE IF NOT EXISTS tag_rule_samples (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model VARCHAR(255) NOT NULL,
    name VARCHAR(128) NOT NULL,
    input JSONB NOT NULL,
    description VARCHAR(512) NOT NULL DEFAULT '',
    CONSTRAINT uq_tag_rule_samples_model_name UNIQUE (model, name)
);

COMMENT ON COLUMN tag_rule_samples.id IS 'the primary key of the tag rule sample';
COMMENT ON COLUMN tag_rule_samples.created_at IS 'the creation time of the tag rule sample';
COMMENT ON COLUMN tag_rule_samples.updated_at IS 'the last update time of the tag rule sample';
COMMENT ON COLUMN tag_rule_samples.model IS 'the target model type key, e.g. zhongkui.com/zhongkui/internal/models/network.HttpTransport';
COMMENT ON COLUMN tag_rule_samples.name IS 'the unique name of the sample within the model';
COMMENT ON COLUMN tag_rule_samples.input IS 'the sample input document, a JSON serialization of the target model instance';
COMMENT ON COLUMN tag_rule_samples.description IS 'the description of the sample';

-- TagRuleSample fixtures: HttpTransport 模型验证样例（覆盖典型请求形态，供运行期合法性校验使用）
INSERT INTO tag_rule_samples (model, name, input, description) VALUES
    (
        'zhongkui.com/zhongkui/internal/models/network.HttpTransport',
        'curl-get',
        '{"namespace":"default","capture":"eth0","version":"v1","session_id":"3f2504e0-4f89-11d3-9a0c-0305e82c3301","timestamp":"2026-08-15T02:01:00Z","created_at":"2026-08-15T02:01:00Z","trace_id":"6ba7b810-9dad-11d1-80b4-00c04fd430c8","client_ip":"192.168.1.10","client_port":51820,"client_isn":1820150330,"server_ip":"93.184.216.34","server_port":443,"server_isn":3896702403,"ip_version":4,"http_version":"HTTP/1.1","stream_index":3,"method":"GET","url":"https://example.com/api/v1/health","scheme":"https","host":"example.com","path":"/api/v1/health","queries":{"verbose":["1"]},"req_body":"","req_body_hash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","req_body_length":0,"req_hdr_user_agent":"curl/8.0.1","req_hdr_host":"example.com","req_hdr_accept":"*/*","req_hdr_authorization":{},"req_hdr_cookie":{},"fp_http_browser":"curl","fp_tcp_client_os":"Linux","fp_tcp_client_sig":"65535:120:1:60:M1460,S,T,N,W8","fp_tcp_mtu":1460,"tags":[]}',
        'command line curl GET request to a health check endpoint'
    ),
    (
        'zhongkui.com/zhongkui/internal/models/network.HttpTransport',
        'browser-post',
        '{"namespace":"default","capture":"en0","version":"v1","session_id":"c81d4e2e-bcf2-11e6-869b-7df92533d2db","timestamp":"2026-08-15T03:12:34Z","created_at":"2026-08-15T03:12:34Z","trace_id":"f47ac10b-58cc-4372-a567-0e02b2c3d479","client_ip":"10.0.0.55","client_port":53210,"client_isn":2147483648,"server_ip":"142.250.72.14","server_port":443,"server_isn":1500000000,"ip_version":4,"http_version":"HTTP/2","stream_index":7,"method":"POST","url":"https://api.example.com/v1/orders","scheme":"https","host":"api.example.com","path":"/v1/orders","queries":{},"req_body":"body/request/9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08.zkc","req_body_hash":"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08","req_body_length":42,"req_hdr_user_agent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36","req_hdr_host":"api.example.com","req_hdr_accept":"application/json","req_hdr_content_type":"application/json","req_hdr_content_length":42,"req_hdr_origin":"https://example.com","req_hdr_referer":"https://example.com/cart","req_hdr_authorization":{},"req_hdr_cookie":{},"fp_http_browser":"Chrome","fp_http_browser_quality":0.98,"fp_http_server":"nginx","fp_http_server_quality":0.9,"fp_tcp_client_os":"macOS","fp_tcp_client_sig":"65535:120:2:60:M1460,S,T,N,W8","fp_tcp_server_os":"Linux","fp_tcp_server_sig":"65535:1440:1:60:M1460,S,N","fp_tcp_mtu":1460,"tags":[]}',
        'browser POST request with fingerprints and JSON body to an api endpoint'
    ),
    (
        'zhongkui.com/zhongkui/internal/models/network.HttpTransport',
        'scanner-crawler',
        '{"namespace":"default","capture":"eth0","version":"v1","session_id":"1e8efd8e-6d34-4a1e-b6b1-4f2e1a3c9d0f","timestamp":"2026-08-15T04:45:00Z","created_at":"2026-08-15T04:45:00Z","trace_id":"9f9c1b5e-3f7a-4c2d-8b5e-2a1b3c4d5e6f","client_ip":"203.0.113.7","client_port":40021,"client_isn":1020304050,"server_ip":"192.0.2.10","server_port":80,"server_isn":2030405060,"ip_version":4,"http_version":"HTTP/1.0","stream_index":1,"method":"GET","url":"http://192.0.2.10/wp-login.php?action=login","scheme":"http","host":"","path":"/wp-login.php","file_ext":"php","queries":{"action":["login"]},"req_body":"","req_body_hash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","req_body_length":0,"req_hdr_user_agent":"python-requests/2.31.0","req_hdr_accept":"*/*","req_hdr_authorization":{},"req_hdr_cookie":{},"fp_tcp_client_os":"Windows","fp_tcp_client_sig":"65535:14600:2:64:M1460,S,T,N,W8","fp_tcp_mtu":1460,"tags":[]}',
        'scanner or crawler request with missing host and query params'
    ),
    (
        'zhongkui.com/zhongkui/internal/models/network.HttpTransport',
        'sparse-minimal',
        '{"namespace":"default","session_id":"00000000-0000-0000-0000-000000000000","method":"HEAD","url":"https://example.com/","scheme":"https","host":"example.com","path":"/","client_ip":"192.168.1.1","client_port":51515,"server_ip":"93.184.216.34","server_port":443,"ip_version":4,"http_version":"HTTP/1.1","req_hdr_user_agent":"","req_hdr_authorization":{},"req_hdr_cookie":{},"tags":[]}',
        'sparse request with minimal fields and empty user agent'
    )
ON CONFLICT (model, name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS tag_rule_samples;
DELETE FROM rbac_policy_templates WHERE target LIKE '/api/v1/admin/%';
DELETE FROM rbac_policies WHERE v0 IN ('superuser', 'admin') AND ptype = 'p';
DELETE FROM namespaces WHERE name = 'default';
DELETE FROM groups WHERE name IN ('superuser', 'admin', 'default');
