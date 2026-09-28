-- +goose Up

-- =====================================================================
-- 003 结构对齐：001_init.sql 已合并「文档导入」与「短期/长期记忆」两条分支的
-- 结构，但既有数据库的 goose 版本已到 002，不会重跑 001，因此缺列/缺表。
-- 本文件以幂等 DDL 补齐差异：agent_memories 表（含索引）、agent_sessions 的
-- 记忆三列、knowledge_documents 的召回统计两列。
-- 全新库由 001 建全，本文件对其整体为空操作；既有库据此收敛到与 001 一致的结构。
--
-- 003 schema alignment: 001_init.sql already merges the structure of both the
-- document-import and the short/long-term-memory branches, but existing databases
-- are already at goose version 002 and never re-run 001, so the table and columns
-- are missing. This file backfills the difference with idempotent DDL: the
-- agent_memories table (with its indexes), the three memory columns of
-- agent_sessions, and the two recall-statistics columns of knowledge_documents.
-- Fresh databases are fully created by 001 and this file is a no-op for them;
-- existing databases converge to the structure defined by 001.
-- =====================================================================

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS agent_memories (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    agent_kind VARCHAR(32) NOT NULL,
    session_id BIGINT NOT NULL DEFAULT 0,
    kind VARCHAR(16) NOT NULL DEFAULT 'fact',
    title VARCHAR(256) NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    embedding vector(1536) NOT NULL,
    search tsvector NOT NULL DEFAULT '',
    hash CHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    use_count INT NOT NULL DEFAULT 0,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_agent_memories_namespace_kind
    ON agent_memories (namespace_name, agent_kind);

CREATE INDEX IF NOT EXISTS idx_agent_memories_embedding
    ON agent_memories USING hnsw (embedding vector_cosine_ops);

CREATE INDEX IF NOT EXISTS idx_agent_memories_search
    ON agent_memories USING gin (search);

CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_memories_scope_hash
    ON agent_memories (namespace_name, agent_kind, hash);

COMMENT ON COLUMN agent_memories.id IS 'the primary key of the agent memory';
COMMENT ON COLUMN agent_memories.created_at IS 'the creation time of the agent memory';
COMMENT ON COLUMN agent_memories.updated_at IS 'the last update time of the agent memory';
COMMENT ON COLUMN agent_memories.deleted_at IS 'the soft delete time of the agent memory, null means not deleted';
COMMENT ON COLUMN agent_memories.namespace_name IS 'the namespace the agent memory belongs to';
COMMENT ON COLUMN agent_memories.agent_kind IS 'the agent kind the memory belongs to, e.g. knowledge or rego';
COMMENT ON COLUMN agent_memories.session_id IS 'the originating session id, 0 for memories added manually (no cascade so memories survive session deletion)';
COMMENT ON COLUMN agent_memories.kind IS 'the category of the memory, e.g. fact, preference, decision or artifact';
COMMENT ON COLUMN agent_memories.title IS 'the one-line title of the memory';
COMMENT ON COLUMN agent_memories.content IS 'the body of the memory';
COMMENT ON COLUMN agent_memories.embedding IS 'the vectorized representation of the memory content for similarity retrieval';
COMMENT ON COLUMN agent_memories.search IS 'the full-text search representation of the memory content';
COMMENT ON COLUMN agent_memories.hash IS 'the sha256 hex digest of the normalized memory content, used for exact deduplication';
COMMENT ON COLUMN agent_memories.status IS 'the status of the memory, e.g. active or archived';
COMMENT ON COLUMN agent_memories.use_count IS 'the number of times the memory has been recalled by retrieval, default 0';
COMMENT ON COLUMN agent_memories.last_used_at IS 'the last time the memory was recalled by retrieval, null when never used';

ALTER TABLE agent_sessions ADD COLUMN IF NOT EXISTS agent_kind VARCHAR(32) NOT NULL DEFAULT 'knowledge';
ALTER TABLE agent_sessions ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_sessions ADD COLUMN IF NOT EXISTS summary_upto_message_id BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN agent_sessions.agent_kind IS 'the agent kind the session belongs to, e.g. knowledge or rego';
COMMENT ON COLUMN agent_sessions.summary IS 'the rolling summary of the session, compressing the messages already folded into it, empty when none';
COMMENT ON COLUMN agent_sessions.summary_upto_message_id IS 'the id of the last message covered by the rolling summary, 0 means nothing is summarized yet';

ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS use_count INT NOT NULL DEFAULT 0;
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;

COMMENT ON COLUMN knowledge_documents.use_count IS 'the number of times the document has been recalled by retrieval, one increment per retrieval hitting any of its chunks, default 0';
COMMENT ON COLUMN knowledge_documents.last_used_at IS 'the last time the document was recalled by retrieval, null when never used';

-- +goose Down
ALTER TABLE knowledge_documents DROP COLUMN IF EXISTS last_used_at;
ALTER TABLE knowledge_documents DROP COLUMN IF EXISTS use_count;
ALTER TABLE agent_sessions DROP COLUMN IF EXISTS summary_upto_message_id;
ALTER TABLE agent_sessions DROP COLUMN IF EXISTS summary;
ALTER TABLE agent_sessions DROP COLUMN IF EXISTS agent_kind;
DROP TABLE IF EXISTS agent_memories;
