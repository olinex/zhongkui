-- +goose Up

-- =====================================================================
-- 全局种子数据与权限点（fixtures）
-- 本文件合并迁移 002~008 的 fixtures 内容，无向后兼容负担：
-- 权限点模板已合并补齐全部需认证接口且移除已废弃的 jwt/refresh；
-- 标签规则样例表 DDL 与种子数据随本文件一起迁移（无外键依赖）；
-- Agent 行为参数（含提示词）的出厂值在此种子化，作为参数类值的唯一来源。
-- =====================================================================

-- -------------------------------------------------------------
-- 1) 用户组
-- -------------------------------------------------------------
INSERT INTO groups (name, description) VALUES
    ('superuser', 'superuser group users who have all right to do anything'),
    ('admin', 'admin group users who have the most right to do very much things'),
    ('default', 'default group users who have all default right')
ON CONFLICT (name) DO NOTHING;

-- -------------------------------------------------------------
-- 2) 命名空间
-- -------------------------------------------------------------
INSERT INTO namespaces (name, description) VALUES
    ('default', 'default namespace')
ON CONFLICT (name) DO NOTHING;

-- -------------------------------------------------------------
-- 3) RBAC 策略：superuser / admin
-- -------------------------------------------------------------
-- superuser 通配放行
INSERT INTO rbac_policies (ptype, v0, v1, v2, v3) VALUES
    ('p', 'superuser', '*', '*', 'allow')
ON CONFLICT (ptype, v0, v1, v2) DO NOTHING;

-- admin 组策略
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
    ('p', 'admin', 'api:/api/v1/admin/tag-rule-sample/delete', 'OPTIONS', 'allow'),
    -- 第三方数据源接口
    ('p', 'admin', 'api:/api/v1/datasource/types', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/types', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/create', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/create', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/:id/get', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/:id/get', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/:id/update', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/:id/update', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/delete', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/delete', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/enable', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/enable', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/disable', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/disable', 'OPTIONS', 'allow'),
    -- 第三方数据源：连接测试与结构自省
    ('p', 'admin', 'api:/api/v1/datasource/test', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/test', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/introspect', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/datasource/introspect', 'OPTIONS', 'allow'),
    -- 管理接口：服务/依赖状态、采集器运行状态与统一枚举
    ('p', 'admin', 'api:/api/v1/admin/service/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/service/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/deps/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/deps/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/capturer/status/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/capturer/status/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/enum/get', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/enum/get', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/enum/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/enum/list', 'OPTIONS', 'allow'),
    -- 标签事件接口：OPA 内置函数与 sqlite schema 元信息
    ('p', 'admin', 'api:/api/v1/tag-event/tag-rule/functions', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/tag-event/tag-rule/functions', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/tag-event/tag-rule/sqlite/meta', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/tag-event/tag-rule/sqlite/meta', 'OPTIONS', 'allow'),
    -- 原始流量审计接口：行详情
    ('p', 'admin', 'api:/api/v1/traffic/ipv4-tcp/detail', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/traffic/ipv4-tcp/detail', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/traffic/ipv6-tcp/detail', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/traffic/ipv6-tcp/detail', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/traffic/http-transport/detail', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/traffic/http-transport/detail', 'OPTIONS', 'allow'),
    -- 通知模板：事件元信息
    ('p', 'admin', 'api:/api/v1/notification/template/events/meta', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/notification/template/events/meta', 'OPTIONS', 'allow'),
    -- AI 助手接口（知识库文档 / Agent 会话 / 流式生成 / 状态）
    ('p', 'admin', 'api:/api/v1/ai/doc/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/create', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/create', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/:id/get', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/:id/get', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/:id/update', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/:id/update', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/delete', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/delete', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/:id/reindex', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/doc/:id/reindex', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/create', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/create', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/:id/get', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/:id/get', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/:id/rename', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/:id/rename', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/:id/delete', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/:id/messages', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/session/:id/messages', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/agent/:sessionID/stream', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/agent/:sessionID/stream', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/status', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/status', 'OPTIONS', 'allow'),
    -- AI 助手 admin 配置接口（模型注册表 / 档位绑定 / Agent 参数覆盖）
    ('p', 'admin', 'api:/api/v1/admin/llm/model/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/create', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/create', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/:id/get', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/:id/get', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/:id/update', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/:id/update', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/delete', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/delete', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/test', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/model/test', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/binding/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/binding/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/binding/:slot/set', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/binding/:slot/set', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/binding/:slot/unbind', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/binding/:slot/unbind', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/param/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/param/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/param/set', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/param/set', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/param/reset', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/admin/llm/param/reset', 'OPTIONS', 'allow')
ON CONFLICT (ptype, v0, v1, v2) DO NOTHING;

-- -------------------------------------------------------------
-- 4) RBAC 策略模板
-- -------------------------------------------------------------
-- 接口层模板：target = api:{FullPath} + action = HTTP 方法（POST / OPTIONS），覆盖全部需认证接口；
-- 明文查看权限点模板：target = plaintext:{crypto|s3}:{model}:{field} + action = READ
-- （格式规范见 internal/pkg/base/consts.go），供 group/:name/add-rbac-policy 校验 target 合法范围。
-- jwt/refresh 豁免强制认证与授权，不注册模板。
INSERT INTO rbac_policy_templates (target, action) VALUES
    -- 管理接口：用户管理
    ('api:/api/v1/admin/user/list', 'POST'),
    ('api:/api/v1/admin/user/list', 'OPTIONS'),
    ('api:/api/v1/admin/user/create', 'POST'),
    ('api:/api/v1/admin/user/create', 'OPTIONS'),
    ('api:/api/v1/admin/user/:unionID/get', 'POST'),
    ('api:/api/v1/admin/user/:unionID/get', 'OPTIONS'),
    ('api:/api/v1/admin/user/:unionID/update', 'POST'),
    ('api:/api/v1/admin/user/:unionID/update', 'OPTIONS'),
    ('api:/api/v1/admin/user/:unionID/password/reset', 'POST'),
    ('api:/api/v1/admin/user/:unionID/password/reset', 'OPTIONS'),
    ('api:/api/v1/admin/user/:unionID/frontend-config/reset', 'POST'),
    ('api:/api/v1/admin/user/:unionID/frontend-config/reset', 'OPTIONS'),
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
    -- 管理接口：服务/依赖状态与统一枚举
    ('api:/api/v1/admin/service/list', 'POST'),
    ('api:/api/v1/admin/service/list', 'OPTIONS'),
    ('api:/api/v1/admin/deps/list', 'POST'),
    ('api:/api/v1/admin/deps/list', 'OPTIONS'),
    ('api:/api/v1/admin/enum/get', 'POST'),
    ('api:/api/v1/admin/enum/get', 'OPTIONS'),
    ('api:/api/v1/admin/enum/list', 'POST'),
    ('api:/api/v1/admin/enum/list', 'OPTIONS'),
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
    -- 管理接口：采集器运行状态
    ('api:/api/v1/admin/capturer/status/list', 'POST'),
    ('api:/api/v1/admin/capturer/status/list', 'OPTIONS'),
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
    ('api:/api/v1/tag-event/tag-rule/model/metadata', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/model/metadata', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/model/samples', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/model/samples', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/functions', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/functions', 'OPTIONS'),
    ('api:/api/v1/tag-event/tag-rule/sqlite/meta', 'POST'),
    ('api:/api/v1/tag-event/tag-rule/sqlite/meta', 'OPTIONS'),
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
    -- 标签事件接口：事件记录
    ('api:/api/v1/tag-event/record/list', 'POST'),
    ('api:/api/v1/tag-event/record/list', 'OPTIONS'),
    ('api:/api/v1/tag-event/record/:id/get', 'POST'),
    ('api:/api/v1/tag-event/record/:id/get', 'OPTIONS'),
    ('api:/api/v1/tag-event/record/close', 'POST'),
    ('api:/api/v1/tag-event/record/close', 'OPTIONS'),
    ('api:/api/v1/tag-event/record/hit/records', 'POST'),
    ('api:/api/v1/tag-event/record/hit/records', 'OPTIONS'),
    -- 第三方数据源接口
    ('api:/api/v1/datasource/types', 'POST'),
    ('api:/api/v1/datasource/types', 'OPTIONS'),
    ('api:/api/v1/datasource/list', 'POST'),
    ('api:/api/v1/datasource/list', 'OPTIONS'),
    ('api:/api/v1/datasource/create', 'POST'),
    ('api:/api/v1/datasource/create', 'OPTIONS'),
    ('api:/api/v1/datasource/test', 'POST'),
    ('api:/api/v1/datasource/test', 'OPTIONS'),
    ('api:/api/v1/datasource/introspect', 'POST'),
    ('api:/api/v1/datasource/introspect', 'OPTIONS'),
    ('api:/api/v1/datasource/:id/get', 'POST'),
    ('api:/api/v1/datasource/:id/get', 'OPTIONS'),
    ('api:/api/v1/datasource/:id/update', 'POST'),
    ('api:/api/v1/datasource/:id/update', 'OPTIONS'),
    ('api:/api/v1/datasource/delete', 'POST'),
    ('api:/api/v1/datasource/delete', 'OPTIONS'),
    ('api:/api/v1/datasource/enable', 'POST'),
    ('api:/api/v1/datasource/enable', 'OPTIONS'),
    ('api:/api/v1/datasource/disable', 'POST'),
    ('api:/api/v1/datasource/disable', 'OPTIONS'),
    -- 原始流量审计接口
    ('api:/api/v1/traffic/ipv4-tcp/list', 'POST'),
    ('api:/api/v1/traffic/ipv4-tcp/list', 'OPTIONS'),
    ('api:/api/v1/traffic/ipv4-tcp/detail', 'POST'),
    ('api:/api/v1/traffic/ipv4-tcp/detail', 'OPTIONS'),
    ('api:/api/v1/traffic/ipv6-tcp/list', 'POST'),
    ('api:/api/v1/traffic/ipv6-tcp/list', 'OPTIONS'),
    ('api:/api/v1/traffic/ipv6-tcp/detail', 'POST'),
    ('api:/api/v1/traffic/ipv6-tcp/detail', 'OPTIONS'),
    ('api:/api/v1/traffic/http-transport/list', 'POST'),
    ('api:/api/v1/traffic/http-transport/list', 'OPTIONS'),
    ('api:/api/v1/traffic/http-transport/detail', 'POST'),
    ('api:/api/v1/traffic/http-transport/detail', 'OPTIONS'),
    ('api:/api/v1/traffic/http-transport/stack/list', 'POST'),
    ('api:/api/v1/traffic/http-transport/stack/list', 'OPTIONS'),
    -- 通用明文查看接口
    ('api:/api/v1/plaintext/crypto', 'POST'),
    ('api:/api/v1/plaintext/crypto', 'OPTIONS'),
    ('api:/api/v1/plaintext/s3', 'POST'),
    ('api:/api/v1/plaintext/s3', 'OPTIONS'),
    -- AI 助手接口（知识库文档 / Agent 会话 / 流式生成 / 状态）
    ('api:/api/v1/ai/doc/list', 'POST'),
    ('api:/api/v1/ai/doc/list', 'OPTIONS'),
    ('api:/api/v1/ai/doc/create', 'POST'),
    ('api:/api/v1/ai/doc/create', 'OPTIONS'),
    ('api:/api/v1/ai/doc/:id/get', 'POST'),
    ('api:/api/v1/ai/doc/:id/get', 'OPTIONS'),
    ('api:/api/v1/ai/doc/:id/update', 'POST'),
    ('api:/api/v1/ai/doc/:id/update', 'OPTIONS'),
    ('api:/api/v1/ai/doc/delete', 'POST'),
    ('api:/api/v1/ai/doc/delete', 'OPTIONS'),
    ('api:/api/v1/ai/doc/:id/reindex', 'POST'),
    ('api:/api/v1/ai/doc/:id/reindex', 'OPTIONS'),
    ('api:/api/v1/ai/session/list', 'POST'),
    ('api:/api/v1/ai/session/create', 'POST'),
    ('api:/api/v1/ai/session/create', 'OPTIONS'),
    ('api:/api/v1/ai/session/:id/get', 'POST'),
    ('api:/api/v1/ai/session/:id/get', 'OPTIONS'),
    ('api:/api/v1/ai/session/:id/rename', 'POST'),
    ('api:/api/v1/ai/session/:id/rename', 'OPTIONS'),
    ('api:/api/v1/ai/session/:id/delete', 'POST'),
    ('api:/api/v1/ai/session/:id/messages', 'POST'),
    ('api:/api/v1/ai/session/:id/messages', 'OPTIONS'),
    ('api:/api/v1/ai/agent/:sessionID/stream', 'POST'),
    ('api:/api/v1/ai/agent/:sessionID/stream', 'OPTIONS'),
    ('api:/api/v1/ai/status', 'POST'),
    ('api:/api/v1/ai/status', 'OPTIONS'),
    -- AI 助手 admin 配置接口（模型注册表 / 档位绑定 / Agent 参数覆盖）
    ('api:/api/v1/admin/llm/model/list', 'POST'),
    ('api:/api/v1/admin/llm/model/list', 'OPTIONS'),
    ('api:/api/v1/admin/llm/model/create', 'POST'),
    ('api:/api/v1/admin/llm/model/create', 'OPTIONS'),
    ('api:/api/v1/admin/llm/model/:id/get', 'POST'),
    ('api:/api/v1/admin/llm/model/:id/get', 'OPTIONS'),
    ('api:/api/v1/admin/llm/model/:id/update', 'POST'),
    ('api:/api/v1/admin/llm/model/:id/update', 'OPTIONS'),
    ('api:/api/v1/admin/llm/model/delete', 'POST'),
    ('api:/api/v1/admin/llm/model/delete', 'OPTIONS'),
    ('api:/api/v1/admin/llm/model/test', 'POST'),
    ('api:/api/v1/admin/llm/model/test', 'OPTIONS'),
    ('api:/api/v1/admin/llm/binding/list', 'POST'),
    ('api:/api/v1/admin/llm/binding/list', 'OPTIONS'),
    ('api:/api/v1/admin/llm/binding/:slot/set', 'POST'),
    ('api:/api/v1/admin/llm/binding/:slot/set', 'OPTIONS'),
    ('api:/api/v1/admin/llm/binding/:slot/unbind', 'POST'),
    ('api:/api/v1/admin/llm/binding/:slot/unbind', 'OPTIONS'),
    ('api:/api/v1/admin/llm/param/list', 'POST'),
    ('api:/api/v1/admin/llm/param/list', 'OPTIONS'),
    ('api:/api/v1/admin/llm/param/set', 'POST'),
    ('api:/api/v1/admin/llm/param/set', 'OPTIONS'),
    ('api:/api/v1/admin/llm/param/reset', 'POST'),
    ('api:/api/v1/admin/llm/param/reset', 'OPTIONS'),
    -- 个人接口（default 组自管资源）
    ('api:/api/v1/user/self/password/reset', 'POST'),
    ('api:/api/v1/user/self/password/reset', 'OPTIONS'),
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
    ('api:/api/v1/user/self/frontend-config/get', 'POST'),
    ('api:/api/v1/user/self/frontend-config/get', 'OPTIONS'),
    ('api:/api/v1/user/self/frontend-config/sync', 'POST'),
    ('api:/api/v1/user/self/frontend-config/sync', 'OPTIONS'),
    ('api:/api/v1/user/self/frontend-config/sync-enabled/set', 'POST'),
    ('api:/api/v1/user/self/frontend-config/sync-enabled/set', 'OPTIONS'),
    -- 通知管理：渠道
    ('api:/api/v1/notification/channel/types', 'POST'),
    ('api:/api/v1/notification/channel/types', 'OPTIONS'),
    ('api:/api/v1/notification/channel/list', 'POST'),
    ('api:/api/v1/notification/channel/list', 'OPTIONS'),
    ('api:/api/v1/notification/channel/create', 'POST'),
    ('api:/api/v1/notification/channel/create', 'OPTIONS'),
    ('api:/api/v1/notification/channel/:id/get', 'POST'),
    ('api:/api/v1/notification/channel/:id/get', 'OPTIONS'),
    ('api:/api/v1/notification/channel/:id/update', 'POST'),
    ('api:/api/v1/notification/channel/:id/update', 'OPTIONS'),
    ('api:/api/v1/notification/channel/delete', 'POST'),
    ('api:/api/v1/notification/channel/delete', 'OPTIONS'),
    ('api:/api/v1/notification/channel/enable', 'POST'),
    ('api:/api/v1/notification/channel/enable', 'OPTIONS'),
    ('api:/api/v1/notification/channel/disable', 'POST'),
    ('api:/api/v1/notification/channel/disable', 'OPTIONS'),
    -- 通知管理：目标
    ('api:/api/v1/notification/target/list', 'POST'),
    ('api:/api/v1/notification/target/list', 'OPTIONS'),
    ('api:/api/v1/notification/target/create', 'POST'),
    ('api:/api/v1/notification/target/create', 'OPTIONS'),
    ('api:/api/v1/notification/target/:id/get', 'POST'),
    ('api:/api/v1/notification/target/:id/get', 'OPTIONS'),
    ('api:/api/v1/notification/target/:id/update', 'POST'),
    ('api:/api/v1/notification/target/:id/update', 'OPTIONS'),
    ('api:/api/v1/notification/target/:id/test-send', 'POST'),
    ('api:/api/v1/notification/target/:id/test-send', 'OPTIONS'),
    ('api:/api/v1/notification/target/:id/send', 'POST'),
    ('api:/api/v1/notification/target/:id/send', 'OPTIONS'),
    ('api:/api/v1/notification/target/delete', 'POST'),
    ('api:/api/v1/notification/target/delete', 'OPTIONS'),
    ('api:/api/v1/notification/target/enable', 'POST'),
    ('api:/api/v1/notification/target/enable', 'OPTIONS'),
    ('api:/api/v1/notification/target/disable', 'POST'),
    ('api:/api/v1/notification/target/disable', 'OPTIONS'),
    -- 通知管理：组
    ('api:/api/v1/notification/group/list', 'POST'),
    ('api:/api/v1/notification/group/list', 'OPTIONS'),
    ('api:/api/v1/notification/group/create', 'POST'),
    ('api:/api/v1/notification/group/create', 'OPTIONS'),
    ('api:/api/v1/notification/group/:id/get', 'POST'),
    ('api:/api/v1/notification/group/:id/get', 'OPTIONS'),
    ('api:/api/v1/notification/group/:id/update', 'POST'),
    ('api:/api/v1/notification/group/:id/update', 'OPTIONS'),
    ('api:/api/v1/notification/group/:id/set-default', 'POST'),
    ('api:/api/v1/notification/group/:id/set-default', 'OPTIONS'),
    ('api:/api/v1/notification/group/delete', 'POST'),
    ('api:/api/v1/notification/group/delete', 'OPTIONS'),
    -- 通知管理：模板
    ('api:/api/v1/notification/template/list', 'POST'),
    ('api:/api/v1/notification/template/list', 'OPTIONS'),
    ('api:/api/v1/notification/template/events/meta', 'POST'),
    ('api:/api/v1/notification/template/events/meta', 'OPTIONS'),
    ('api:/api/v1/notification/template/create', 'POST'),
    ('api:/api/v1/notification/template/create', 'OPTIONS'),
    ('api:/api/v1/notification/template/update', 'POST'),
    ('api:/api/v1/notification/template/update', 'OPTIONS'),
    ('api:/api/v1/notification/template/restore-default', 'POST'),
    ('api:/api/v1/notification/template/restore-default', 'OPTIONS'),
    ('api:/api/v1/notification/template/render-test', 'POST'),
    ('api:/api/v1/notification/template/render-test', 'OPTIONS'),
    ('api:/api/v1/notification/template/validate', 'POST'),
    ('api:/api/v1/notification/template/validate', 'OPTIONS'),
    ('api:/api/v1/notification/template/dry-run', 'POST'),
    ('api:/api/v1/notification/template/dry-run', 'OPTIONS'),
    -- 通知管理：发送记录
    ('api:/api/v1/notification/record/list', 'POST'),
    ('api:/api/v1/notification/record/list', 'OPTIONS'),
    ('api:/api/v1/notification/record/:id/get', 'POST'),
    ('api:/api/v1/notification/record/:id/get', 'OPTIONS'),
    -- ETL 管道：监控接口（inspector）
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
    -- ETL 管道：定时任务管理
    ('api:/api/v1/etl/scheduler/task/create', 'POST'),
    ('api:/api/v1/etl/scheduler/task/create', 'OPTIONS'),
    ('api:/api/v1/etl/scheduler/task/update', 'POST'),
    ('api:/api/v1/etl/scheduler/task/update', 'OPTIONS'),
    ('api:/api/v1/etl/scheduler/task/toggle', 'POST'),
    ('api:/api/v1/etl/scheduler/task/toggle', 'OPTIONS'),
    ('api:/api/v1/etl/scheduler/task/delete', 'POST'),
    ('api:/api/v1/etl/scheduler/task/delete', 'OPTIONS'),
    ('api:/api/v1/etl/scheduler/task/run', 'POST'),
    ('api:/api/v1/etl/scheduler/task/run', 'OPTIONS'),
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
    ('plaintext:crypto:datasource:config', 'READ'),
    ('plaintext:s3:http-transport:reqBody', 'READ'),
    ('plaintext:s3:http-transport:rspBody', 'READ')
ON CONFLICT (target, action) DO NOTHING;

-- -------------------------------------------------------------
-- 5) default 组默认策略：普通用户对 user/self 自管资源拥有全部权限
-- -------------------------------------------------------------
-- 生效域由用户创建时绑定的 g(user, default, default) 限定在 default 命名空间；
-- jwt/refresh 已豁免授权，不在其中。
INSERT INTO rbac_policies (ptype, v0, v1, v2, v3) VALUES
    ('p', 'default', 'api:/api/v1/user/self/password/reset', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/password/reset', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/authorization-info/get', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/authorization-info/get', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/profile/change', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/profile/change', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/list', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/list', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/create', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/create', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/:id/remove', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/:id/remove', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/:id/activate', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/:id/activate', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/:id/deactivate', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/access-key/:id/deactivate', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/frontend-config/get', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/frontend-config/get', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/frontend-config/sync', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/frontend-config/sync', 'OPTIONS', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/frontend-config/sync-enabled/set', 'POST', 'allow'),
    ('p', 'default', 'api:/api/v1/user/self/frontend-config/sync-enabled/set', 'OPTIONS', 'allow')
ON CONFLICT (ptype, v0, v1, v2) DO NOTHING;

-- -------------------------------------------------------------
-- 6) 标签规则样例
-- -------------------------------------------------------------
-- 建表语句见 001_init.sql（schema 层），此处仅种入基线样例数据。
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

-- -------------------------------------------------------------
-- 7) 占位符通知渠道
-- -------------------------------------------------------------
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

-- -------------------------------------------------------------
-- 8) Agent 参数出厂值
-- -------------------------------------------------------------
-- 参数键由代码内注册表（internal/pkg/llm 的 Specs）声明，参数类值不写在代码里：出厂值
-- 只在此段种子化，value 与 default_value 初始相同。value 是运行期生效值（可由管理
-- 接口或直接改库调整），default_value 是只读出厂值（供「恢复出厂值」写回）。启动期强校验
-- 要求注册表每个键都有库内记录，故代码内新增参数必须同步补此段。
-- 提示词正文含单引号与 JSON 双引号，统一用美元引用（$$...$$）包裹，避免转义。
INSERT INTO agent_params (key, value, default_value)
SELECT v.key, v.value, v.value
FROM (VALUES
    (
        'agent.systemPrompt',
        $$You are "ZhongKui AI", an embedded assistant of a network-traffic capture and analysis platform. You help users inspect captured packets, build tag rules, query data sources, and operate the platform.

You must reply with ONLY a JSON object in exactly this shape:
{"content": "<your markdown answer>", "draft": {<optional structured payload>}}

Rules:
- "content": the visible answer in Markdown. Match the language of the user's question.
- "draft": a JSON object capturing structured facts, extracted entities or actionable plans for downstream automation; use {} when nothing structured applies.
- Answer questions using the retrieved knowledge when relevant and cite sources by their titles.
- Never fabricate facts beyond the provided context.$$
    ),
    ('agent.retrievalSectionTitle', '## Retrieved Knowledge'),
    ('agent.businessSectionTitle', '## Current Business Scene'),
    ('agent.questionSectionTitle', '## User Question'),
    ('agent.retrievedDocSnippetLimit', '500'),
    ('agent.defaultSlot', 'standard'),
    ('retrieval.topK', '5'),
    ('retrieval.candidates', '30'),
    ('retrieval.fusion', 'rrf'),
    ('retrieval.weightVector', '0.6'),
    ('retrieval.weightKeyword', '0.4'),
    (
        'retrieval.rerankSystemPrompt',
        $$You are a relevance reranker for retrieval-augmented generation. Rank the given chunks by relevance to the query and return ONLY a JSON array of chunk indices ordered from most to least relevant, e.g. [3,1,0]. Do not output any other text.$$
    ),
    ('retrieval.rerankSnippetLimit', '500'),
    ('retrieval.rerankTemperature', '0.2'),
    ('session.maxContextMessages', '20'),
    ('session.maxContextBytes', '32768'),
    ('chunking.size', '800'),
    ('chunking.overlap', '100'),
    (
        'eval.answerSystemPrompt',
        $$You are a helpful assistant for retrieval-augmented generation. Answer the user's question using ONLY the provided context chunks. If the context does not contain the answer, say that you do not know. Return ONLY a JSON object like {"answer": "..."} and nothing else.$$
    ),
    (
        'eval.chunkRelevanceSystemPrompt',
        $$You are a retrieval quality judge. Given the query and the retrieved context chunks, judge each chunk's relevance to the query. Return ONLY a JSON object like {"judgments": [{"index": 0, "relevant": true, "score": 0.9, "reason": "..."}]} where index is the chunk's number, score is a relevance score between 0 and 1, and reason is a short English explanation. Do not output any other text.$$
    ),
    (
        'eval.claimsSystemPrompt',
        $$You are a faithfulness judge. Extract every factual claim from the given answer and determine whether each claim can be inferred from the provided context chunks. Return ONLY a JSON object like {"claims": [{"statement": "...", "supported": true, "chunkIndexes": [0, 2]}]} where chunkIndexes lists the chunk numbers supporting the claim and is an empty array when the claim is not supported. Do not output any other text.$$
    ),
    (
        'eval.reverseQuestionsSystemPrompt',
        $$You are an answer relevance judge. Reverse-engineer up to 3 questions that the given answer could plausibly answer. Return ONLY a JSON object like {"questions": ["...", "..."]}. Do not output any other text.$$
    ),
    ('eval.reverseQuestionCount', '3'),
    ('eval.snippetLimit', '500'),
    ('sanitize.maxItems', '0'),
    ('sanitize.maxBytes', '32768')
) AS v(key, value)
ON CONFLICT (key) DO NOTHING;

-- +goose Down

-- Agent 参数出厂值
DELETE FROM agent_params;

-- 占位符通知渠道
DELETE FROM notification_channels
WHERE type IN ('webhook')
  AND namespace_name IN (SELECT name FROM namespaces);

-- default 组 user/self 自管策略
DELETE FROM rbac_policies WHERE ptype = 'p' AND v0 = 'default' AND v1 LIKE 'api:/api/v1/user/self/%';

-- RBAC 策略模板
DELETE FROM rbac_policy_templates WHERE target LIKE 'plaintext:%';
DELETE FROM rbac_policy_templates WHERE target LIKE 'api:%';

-- admin / superuser 策略
DELETE FROM rbac_policies WHERE v0 IN ('superuser', 'admin') AND ptype = 'p';

-- 命名空间
DELETE FROM namespaces WHERE name = 'default';

-- 用户组
DELETE FROM groups WHERE name IN ('superuser', 'admin', 'default');
