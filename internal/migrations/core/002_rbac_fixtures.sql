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
-- 接口层权限点 object = api:{FullPath} + action = HTTP 方法（规范见 internal/pkg/base/consts.go）
INSERT INTO rbac_policies (ptype, v0, v1, v2, v3) VALUES
    ('p', 'admin', 'api:/api/v1/admin/user/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/user/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/user/activate', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/user/deactivate', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/user/:unionID/join-group', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/user/:unionID/leave-group', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/group/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/group/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/group/:name/get', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/group/:name/add-rbac-policy', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/group/:name/remove-rbac-policy', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/rbac-policy-template/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/rbac-policy-template/list', 'OPTIONS', 'allow'),
    -- 管理接口：标签规则样例
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/create', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/create', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/:id/get', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/:id/get', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/:id/update', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/:id/update', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/delete', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/delete', 'OPTIONS', 'allow')
ON CONFLICT (ptype, v0, v1, v2) DO NOTHING;

-- RbacPolicyTemplate
-- 接口层模板：target = api:{FullPath} + action = HTTP 方法（POST / OPTIONS），覆盖全部需认证接口；
-- 明文查看权限点模板：target = plaintext:{crypto|s3}:{model}:{field} + action = READ
-- （格式规范见 internal/pkg/base/consts.go），供 group/:name/add-rbac-policy 校验 target 合法范围
INSERT INTO rbac_policy_templates (target, action) VALUES
    -- 管理接口：用户管理
    ('api:/api/v1/admin/user/list', 'POST'),
    ('api:/api/v1/admin/user/list', 'OPTIONS'),
    ('api:/api/v1/admin/user/:unionID/get', 'POST'),
    ('api:/api/v1/admin/user/:unionID/get', 'OPTIONS'),
    ('api:/api/v1/admin/user/:unionID/update', 'POST'),
    ('api:/api/v1/admin/user/:unionID/update', 'OPTIONS'),
    ('api:/api/v1/admin/user/activate', 'POST'),
    ('api:/api/v1/admin/user/activate', 'OPTIONS'),
    ('api:/api/v1/admin/user/deactivate', 'POST'),
    ('api:/api/v1/admin/user/deactivate', 'OPTIONS'),
    ('api:/api/v1/admin/user/:unionID/join-group', 'POST'),
    ('api:/api/v1/admin/user/:unionID/join-group', 'OPTIONS'),
    ('api:/api/v1/admin/user/:unionID/leave-group', 'POST'),
    ('api:/api/v1/admin/user/:unionID/leave-group', 'OPTIONS'),
    -- 管理接口：用户组
    ('api:/api/v1/admin/group/list', 'POST'),
    ('api:/api/v1/admin/group/list', 'OPTIONS'),
    ('api:/api/v1/admin/group/create', 'POST'),
    ('api:/api/v1/admin/group/create', 'OPTIONS'),
    ('api:/api/v1/admin/group/:name/get', 'POST'),
    ('api:/api/v1/admin/group/:name/get', 'OPTIONS'),
    ('api:/api/v1/admin/group/:name/update', 'POST'),
    ('api:/api/v1/admin/group/:name/update', 'OPTIONS'),
    ('api:/api/v1/admin/group/delete', 'POST'),
    ('api:/api/v1/admin/group/delete', 'OPTIONS'),
    ('api:/api/v1/admin/group/:name/add-rbac-policy', 'POST'),
    ('api:/api/v1/admin/group/:name/add-rbac-policy', 'OPTIONS'),
    ('api:/api/v1/admin/group/:name/remove-rbac-policy', 'POST'),
    ('api:/api/v1/admin/group/:name/remove-rbac-policy', 'OPTIONS'),
    -- 管理接口：RBAC 策略模板
    ('api:/api/v1/admin/rbac-policy-template/list', 'POST'),
    ('api:/api/v1/admin/rbac-policy-template/list', 'OPTIONS'),
    -- 管理接口：命名空间
    ('api:/api/v1/admin/namespace/list', 'POST'),
    ('api:/api/v1/admin/namespace/list', 'OPTIONS'),
    ('api:/api/v1/admin/namespace/create', 'POST'),
    ('api:/api/v1/admin/namespace/create', 'OPTIONS'),
    ('api:/api/v1/admin/namespace/:name/get', 'POST'),
    ('api:/api/v1/admin/namespace/:name/get', 'OPTIONS'),
    ('api:/api/v1/admin/namespace/:name/update', 'POST'),
    ('api:/api/v1/admin/namespace/:name/update', 'OPTIONS'),
    ('api:/api/v1/admin/namespace/delete', 'POST'),
    ('api:/api/v1/admin/namespace/delete', 'OPTIONS'),
    -- 管理接口：抓包器
    ('api:/api/v1/admin/capturer/list', 'POST'),
    ('api:/api/v1/admin/capturer/list', 'OPTIONS'),
    ('api:/api/v1/admin/capturer/create', 'POST'),
    ('api:/api/v1/admin/capturer/create', 'OPTIONS'),
    ('api:/api/v1/admin/capturer/:id/get', 'POST'),
    ('api:/api/v1/admin/capturer/:id/get', 'OPTIONS'),
    ('api:/api/v1/admin/capturer/:id/update', 'POST'),
    ('api:/api/v1/admin/capturer/:id/update', 'OPTIONS'),
    ('api:/api/v1/admin/capturer/delete', 'POST'),
    ('api:/api/v1/admin/capturer/delete', 'OPTIONS'),
    -- 标签事件接口：标签规则
    ('api:/api/v1/tag-event/tag-rule/list', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/list', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/create', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/create', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/:id/get', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/:id/get', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/:id/update', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/:id/update', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/delete', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/delete', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/enable', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/enable', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/disable', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/disable', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/validate', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/validate', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/single/dry-run', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/single/dry-run', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/group/dry-run', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/group/dry-run', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/:id/version/list', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/:id/version/list', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/:id/version/:version/rollback', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/:id/version/:version/rollback', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/model/options', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/model/options', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/model/metadata', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/model/metadata', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/model/samples', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/model/samples', 'OPTIONS'),
    -- 标签事件接口：标签规则样例
    ('api:/api/v1/tag-event/tag-rule-sample/list', 'POST'),
    ('api:/api/v1/tag-event/tag-rule-sample/list', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule-sample/create', 'POST'),
    ('api:/api/v1/tag-event/tag-rule-sample/create', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule-sample/:id/get', 'POST'),
    ('api:/api/v1/tag-event/tag-rule-sample/:id/get', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule-sample/:id/update', 'POST'),
    ('api:/api/v1/tag-event/tag-rule-sample/:id/update', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule-sample/delete', 'POST'),
    ('api:/api/v1/tag-event/tag-rule-sample/delete', 'OPTIONS'),
    -- 标签事件接口：标签事件规则
    ('api:/api/v1/tag-event/tag-event-rule/list', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/list', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/create', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/create', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/:id/get', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/:id/get', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/:id/update', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/:id/update', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/delete', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/delete', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/enable', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/enable', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/disable', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/disable', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/validate', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/validate', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/dry-run', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/dry-run', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/:id/version/list', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/:id/version/list', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-event-rule/:id/version/:version/rollback', 'POST'),
    ('api:/api/v1/tag-event/tag-event-rule/:id/version/:version/rollback', 'OPTIONS'),
    -- 原始流量审计接口（无唯一性 ID，不提供 detail 接口，list 返回全字段）
    ('api:/api/v1/traffic/ipv4-tcp/list', 'POST'),
    ('api:/api/v1/traffic/ipv4-tcp/list', 'OPTIONS'),
    ('api:/api/v1/traffic/ipv6-tcp/list', 'POST'),
    ('api:/api/v1/traffic/ipv6-tcp/list', 'OPTIONS'),
    ('api:/api/v1/traffic/http-transport/list', 'POST'),
    ('api:/api/v1/traffic/http-transport/list', 'OPTIONS'),
    -- 通用明文查看接口
    ('api:/api/v1/plaintext/crypto', 'POST'),
    ('api:/api/v1/plaintext/crypto', 'OPTIONS'),
    ('api:/api/v1/plaintext/s3', 'POST'),
    ('api:/api/v1/plaintext/s3', 'OPTIONS'),
    -- 个人接口
    ('api:/api/v1/user/self/password/reset', 'POST'),
    ('api:/api/v1/user/self/password/reset', 'OPTIONS'),
    ('api:/api/v1/user/self/jwt/refresh', 'POST'),
    ('api:/api/v1/user/self/jwt/refresh', 'OPTIONS'),
    ('api:/api/v1/user/self/authorization-info/get', 'POST'),
    ('api:/api/v1/user/self/authorization-info/get', 'OPTIONS'),
    ('api:/api/v1/user/self/profile/change', 'POST'),
    ('api:/api/v1/user/self/profile/change', 'OPTIONS'),
    ('api:/api/v1/user/self/access-key/list', 'POST'),
    ('api:/api/v1/user/self/access-key/list', 'OPTIONS'),
    ('api:/api/v1/user/self/access-key/create', 'POST'),
    ('api:/api/v1/user/self/access-key/create', 'OPTIONS'),
    ('api:/api/v1/user/self/access-key/:id/remove', 'POST'),
    ('api:/api/v1/user/self/access-key/:id/remove', 'OPTIONS'),
    ('api:/api/v1/user/self/access-key/:id/activate', 'POST'),
    ('api:/api/v1/user/self/access-key/:id/activate', 'OPTIONS'),
    ('api:/api/v1/user/self/access-key/:id/deactivate', 'POST'),
    ('api:/api/v1/user/self/access-key/:id/deactivate', 'OPTIONS'),
    -- ETL 管道监控接口
    ('api:/api/v1/etl/inspector/server/list', 'POST'),
    ('api:/api/v1/etl/inspector/server/list', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/scheduler/list', 'POST'),
    ('api:/api/v1/etl/inspector/scheduler/list', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/queue/list', 'POST'),
    ('api:/api/v1/etl/inspector/queue/list', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/queue/:name/unpause', 'POST'),
    ('api:/api/v1/etl/inspector/queue/:name/unpause', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/queue/:name/pause', 'POST'),
    ('api:/api/v1/etl/inspector/queue/:name/pause', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/queue/daily-stat/list', 'POST'),
    ('api:/api/v1/etl/inspector/queue/daily-stat/list', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/task/list', 'POST'),
    ('api:/api/v1/etl/inspector/task/list', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/task/state/delete', 'POST'),
    ('api:/api/v1/etl/inspector/task/state/delete', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/task/state/archive', 'POST'),
    ('api:/api/v1/etl/inspector/task/state/archive', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/task/state/run', 'POST'),
    ('api:/api/v1/etl/inspector/task/state/run', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/task/:id/delete', 'POST'),
    ('api:/api/v1/etl/inspector/task/:id/delete', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/task/:id/archive', 'POST'),
    ('api:/api/v1/etl/inspector/task/:id/archive', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/task/:id/run', 'POST'),
    ('api:/api/v1/etl/inspector/task/:id/run', 'OPTIONS'),
    ('api:/api/v1/etl/inspector/task/:id/cancel-processing', 'POST'),
    ('api:/api/v1/etl/inspector/task/:id/cancel-processing', 'OPTIONS'),
    -- 明文查看权限点（数据层）
    ('plaintext:crypto:ipv4-tcp:payload', 'READ'),
    ('plaintext:crypto:ipv6-tcp:payload', 'READ'),
    ('plaintext:crypto:http-transport:reqHdrAuthorization', 'READ'),
    ('plaintext:crypto:http-transport:reqHdrProxyAuthorization', 'READ'),
    ('plaintext:crypto:http-transport:reqHdrCookie', 'READ'),
    ('plaintext:crypto:http-transport:reqHdrSecWebsocketKey', 'READ'),
    ('plaintext:crypto:http-transport:rspHdrWwwAuthenticate', 'READ'),
    ('plaintext:crypto:http-transport:rspHdrProxyAuthenticate', 'READ'),
    ('plaintext:crypto:http-transport:rspHdrSetCookie', 'READ'),
    ('plaintext:crypto:http-transport:rspHdrSecWebsocketAccept', 'READ'),
    ('plaintext:crypto:notification-channel:config', 'READ'),
    ('plaintext:crypto:notification-target:config', 'READ'),
    ('plaintext:s3:http-transport:reqBody', 'READ'),
    ('plaintext:s3:http-transport:rspBody', 'READ')
ON CONFLICT (target, action) DO NOTHING;

-- TagRuleSample: 标签规则验证样例数据（第二层运行期合法性校验用）
CREATE TABLE IF NOT EXISTS tag_rule_samples (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL,
    model VARCHAR(255) NOT NULL,
    name VARCHAR(128) NOT NULL,
    input JSONB NOT NULL,
    description VARCHAR(512) NOT NULL DEFAULT '',
    CONSTRAINT uq_tag_rule_samples_namespace_name_model_name UNIQUE (namespace_name, model, name)
);

COMMENT ON COLUMN tag_rule_samples.id IS 'the primary key of the tag rule sample';
COMMENT ON COLUMN tag_rule_samples.created_at IS 'the creation time of the tag rule sample';
COMMENT ON COLUMN tag_rule_samples.updated_at IS 'the last update time of the tag rule sample';
COMMENT ON COLUMN tag_rule_samples.namespace_name IS 'the namespace the tag rule sample belongs to';
COMMENT ON COLUMN tag_rule_samples.model IS 'the target model type key, e.g. zhongkui.com/zhongkui/internal/models/network.HttpTransport';
COMMENT ON COLUMN tag_rule_samples.name IS 'the unique name of the sample within the namespace and the model';
COMMENT ON COLUMN tag_rule_samples.input IS 'the sample input document, a JSON serialization of the target model instance';
COMMENT ON COLUMN tag_rule_samples.description IS 'the description of the sample';

-- TagRuleSample fixtures: HttpTransport 模型验证样例（覆盖典型请求形态，供运行期合法性校验使用）
-- 种子样例归属 default 命名空间；其它命名空间由 tagengine EnsureModelSamples 懒供给
INSERT INTO tag_rule_samples (namespace_name, model, name, input, description) VALUES
    (
        'default',
        'zhongkui.com/zhongkui/internal/models/network.HttpTransport',
        'curl-get',
        '{"namespace":"default","capture":"eth0","version":"v1","sessionId":"3f2504e0-4f89-11d3-9a0c-0305e82c3301","timestamp":"2026-08-15T02:01:00Z","createdAt":"2026-08-15T02:01:00Z","traceId":"6ba7b810-9dad-11d1-80b4-00c04fd430c8","clientIp":"192.168.1.10","clientPort":51820,"clientIsn":1820150330,"serverIp":"93.184.216.34","serverPort":443,"serverIsn":3896702403,"ipVersion":4,"httpVersion":"HTTP/1.1","streamIndex":3,"method":"GET","url":"https://example.com/api/v1/health","scheme":"https","host":"example.com","path":"/api/v1/health","queries":{"verbose":["1"]},"reqBody":"","reqBodyHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","reqBodyLength":0,"reqHdrUserAgent":"curl/8.0.1","reqHdrHost":"example.com","reqHdrAccept":"*/*","reqHdrAuthorization":{},"reqHdrCookie":{},"fpHttpBrowser":"curl","fpTcpClientOs":"Linux","fpTcpClientSig":"65535:120:1:60:M1460,S,T,N,W8","fpTcpMtu":1460,"tags":[]}',
        'command line curl GET request to a health check endpoint'
    ),
    (
        'default',
        'zhongkui.com/zhongkui/internal/models/network.HttpTransport',
        'browser-post',
        '{"namespace":"default","capture":"en0","version":"v1","sessionId":"c81d4e2e-bcf2-11e6-869b-7df92533d2db","timestamp":"2026-08-15T03:12:34Z","createdAt":"2026-08-15T03:12:34Z","traceId":"f47ac10b-58cc-4372-a567-0e02b2c3d479","clientIp":"10.0.0.55","clientPort":53210,"clientIsn":2147483648,"serverIp":"142.250.72.14","serverPort":443,"serverIsn":1500000000,"ipVersion":4,"httpVersion":"HTTP/2","streamIndex":7,"method":"POST","url":"https://api.example.com/v1/orders","scheme":"https","host":"api.example.com","path":"/v1/orders","queries":{},"reqBody":"body/request/9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08.zkc","reqBodyHash":"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08","reqBodyLength":42,"reqHdrUserAgent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36","reqHdrHost":"api.example.com","reqHdrAccept":"application/json","reqHdrContentType":"application/json","reqHdrContentLength":42,"reqHdrOrigin":"https://example.com","reqHdrReferer":"https://example.com/cart","reqHdrAuthorization":{},"reqHdrCookie":{},"fpHttpBrowser":"Chrome","fpHttpBrowserQuality":0.98,"fpHttpServer":"nginx","fpHttpServerQuality":0.9,"fpTcpClientOs":"macOS","fpTcpClientSig":"65535:120:2:60:M1460,S,T,N,W8","fpTcpServerOs":"Linux","fpTcpServerSig":"65535:1440:1:60:M1460,S,N","fpTcpMtu":1460,"tags":[]}',
        'browser POST request with fingerprints and JSON body to an api endpoint'
    ),
    (
        'default',
        'zhongkui.com/zhongkui/internal/models/network.HttpTransport',
        'scanner-crawler',
        '{"namespace":"default","capture":"eth0","version":"v1","sessionId":"1e8efd8e-6d34-4a1e-b6b1-4f2e1a3c9d0f","timestamp":"2026-08-15T04:45:00Z","createdAt":"2026-08-15T04:45:00Z","traceId":"9f9c1b5e-3f7a-4c2d-8b5e-2a1b3c4d5e6f","clientIp":"203.0.113.7","clientPort":40021,"clientIsn":1020304050,"serverIp":"192.0.2.10","serverPort":80,"serverIsn":2030405060,"ipVersion":4,"httpVersion":"HTTP/1.0","streamIndex":1,"method":"GET","url":"http://192.0.2.10/wp-login.php?action=login","scheme":"http","host":"","path":"/wp-login.php","fileExt":"php","queries":{"action":["login"]},"reqBody":"","reqBodyHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","reqBodyLength":0,"reqHdrUserAgent":"python-requests/2.31.0","reqHdrAccept":"*/*","reqHdrAuthorization":{},"reqHdrCookie":{},"fpTcpClientOs":"Windows","fpTcpClientSig":"65535:14600:2:64:M1460,S,T,N,W8","fpTcpMtu":1460,"tags":[]}',
        'scanner or crawler request with missing host and query params'
    ),
    (
        'default',
        'zhongkui.com/zhongkui/internal/models/network.HttpTransport',
        'sparse-minimal',
        '{"namespace":"default","sessionId":"00000000-0000-0000-0000-000000000000","method":"HEAD","url":"https://example.com/","scheme":"https","host":"example.com","path":"/","clientIp":"192.168.1.1","clientPort":51515,"serverIp":"93.184.216.34","serverPort":443,"ipVersion":4,"httpVersion":"HTTP/1.1","reqHdrUserAgent":"","reqHdrAuthorization":{},"reqHdrCookie":{},"tags":[]}',
        'sparse request with minimal fields and empty user agent'
    )
ON CONFLICT (namespace_name, model, name) DO NOTHING;

-- 占位符通知渠道（如 webhook：渠道级配置无参数，仅标明通知目标类型）作为固定数据
-- 种子化。此处为迁移时已存在的每个命名空间补齐占位渠道（name=type 键、config 置空、
-- enabled=true）；新命名空间的占位渠道由命名空间创建逻辑（code）在其事务内自动种子化。
-- 占位渠道类型键与 notify.PlaceholderTypes() 保持一致，前端不可创建、不可删除。
INSERT INTO notification_channels (namespace_name, name, type, enabled, config, description)
SELECT ns.name, v.type, v.type, true, NULL,
       'builtin placeholder channel: marks the notification target type'
FROM namespaces ns
CROSS JOIN (VALUES ('webhook')) AS v(type)
ON CONFLICT (namespace_name, name) DO NOTHING;

-- +goose Down
DELETE FROM notification_channels
WHERE type IN ('webhook')
  AND namespace_name IN (SELECT name FROM namespaces);
DROP TABLE IF EXISTS tag_rule_samples;
DELETE FROM rbac_policy_templates WHERE target LIKE 'api:%';
DELETE FROM rbac_policy_templates WHERE target LIKE 'plaintext:%';
DELETE FROM rbac_policies WHERE v0 IN ('superuser', 'admin') AND ptype = 'p';
DELETE FROM namespaces WHERE name = 'default';
DELETE FROM groups WHERE name IN ('superuser', 'admin', 'default');
