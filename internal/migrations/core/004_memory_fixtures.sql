-- +goose Up

-- =====================================================================
-- 004 长期记忆的权限点与出参出厂值：002_fixtures.sql 已在原位追加「长期记忆」
-- 相关的 RBAC 权限点与 agent 参数（memory.*）出厂值，但既有数据库的 goose 版本
-- 已到 002，不会重跑 002，故在此以幂等 INSERT 补齐这些缺失行（启动期强校验要求
-- 注册表每个参数键都有库内记录，缺行会导致 AI 运行时装配失败）。
-- 全新库由 002 种下，本文件对其整体为空操作；既有库据此补齐缺失行。
--
-- 004 permissions and parameter defaults of the long-term memory feature:
-- 002_fixtures.sql appended the RBAC permissions and the agent parameter (memory.*)
-- defaults of the feature in place, but existing databases are already at goose
-- version 002 and never re-run 002, so these missing rows are backfilled here with
-- idempotent INSERTs (the startup validation requires a database row for every
-- parameter key in the registry, and a missing row makes the AI runtime fail to
-- assemble). Fresh databases are seeded by 002 and this file is a no-op for them;
-- existing databases gain the missing rows.
-- =====================================================================

-- 长期记忆管理接口的 admin 权限点（列表/新增/详情/更新/批量删除/启停）
INSERT INTO rbac_policies (ptype, v0, v1, v2, v3) VALUES
    ('p', 'admin', 'api:/api/v1/ai/memory/list', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/list', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/create', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/create', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/:id/get', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/:id/get', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/:id/update', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/:id/update', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/delete', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/delete', 'OPTIONS', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/:id/status', 'POST', 'allow'),
    ('p', 'admin', 'api:/api/v1/ai/memory/:id/status', 'OPTIONS', 'allow')
ON CONFLICT (ptype, v0, v1, v2) DO NOTHING;

-- 长期记忆管理接口的权限点模板（供用户组按模板授权）
INSERT INTO rbac_policy_templates (target, action) VALUES
    ('api:/api/v1/ai/memory/list', 'POST'),
    ('api:/api/v1/ai/memory/list', 'OPTIONS'),
    ('api:/api/v1/ai/memory/create', 'POST'),
    ('api:/api/v1/ai/memory/create', 'OPTIONS'),
    ('api:/api/v1/ai/memory/:id/get', 'POST'),
    ('api:/api/v1/ai/memory/:id/get', 'OPTIONS'),
    ('api:/api/v1/ai/memory/:id/update', 'POST'),
    ('api:/api/v1/ai/memory/:id/update', 'OPTIONS'),
    ('api:/api/v1/ai/memory/delete', 'POST'),
    ('api:/api/v1/ai/memory/delete', 'OPTIONS'),
    ('api:/api/v1/ai/memory/:id/status', 'POST'),
    ('api:/api/v1/ai/memory/:id/status', 'OPTIONS')
ON CONFLICT (target, action) DO NOTHING;

-- 长期记忆的 agent 参数出厂值
INSERT INTO agent_params (key, value, default_value)
SELECT v.key, v.value, v.value
FROM (VALUES
    ('memory.enabled', 'true'),
    ('memory.sectionTitle', '## Long-term Memory'),
    ('memory.topK', '5'),
    ('memory.candidates', '20'),
    ('memory.maxItems', '8'),
    ('memory.maxItemBytes', '1024'),
    ('memory.dedupeThreshold', '0.92'),
    (
        'memory.summarySystemPrompt',
        $$You maintain a rolling summary of a long multi-turn conversation between a user and an assistant. Merge the existing summary and the newly evicted messages into one updated summary. Keep it concise, factual and in the language of the conversation. Preserve decisions, constraints, identifiers, names and unresolved questions. Return ONLY the updated summary text, with no preamble.$$
    ),
    ('memory.summaryMinEvictedMessages', '6'),
    (
        'memory.extractSystemPrompt',
        $$You extract durable long-term memories from a conversation between a user and an assistant. Extract only the most reusable items: stable facts, user preferences, decisions and produced artifacts. Each memory must be self-contained and understandable without the conversation. Return ONLY a JSON object like {"memories": [{"kind": "fact", "title": "...", "content": "..."}]} where kind is one of fact, preference, decision, artifact. Return {"memories": []} when nothing is worth remembering.$$
    ),
    ('memory.extractMaxItems', '5')
) AS v(key, value)
ON CONFLICT (key) DO NOTHING;

-- 长期记忆写入工具的内置出厂行（由管理员在工具面板显式启用）
INSERT INTO llm_tools (name, source, mcp_server_id, remote_name, description, enabled)
VALUES ('memory_write', 'builtin', NULL, '', '', false)
ON CONFLICT (name) DO NOTHING;

-- +goose Down

-- 内置工具出厂行
DELETE FROM llm_tools WHERE name = 'memory_write' AND source = 'builtin';

-- agent 参数出厂值
DELETE FROM agent_params WHERE key LIKE 'memory.%';

-- 权限点模板与权限点
DELETE FROM rbac_policy_templates WHERE target LIKE 'api:/api/v1/ai/memory/%';
DELETE FROM rbac_policies WHERE ptype = 'p' AND v1 LIKE 'api:/api/v1/ai/memory/%';
