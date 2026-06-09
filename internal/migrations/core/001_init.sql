-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    union_id UUID NOT NULL UNIQUE,
    nickle_name VARCHAR(64) NOT NULL,
    username VARCHAR(64) NOT NULL UNIQUE,
    password BYTEA NOT NULL,
    email VARCHAR(128) NOT NULL UNIQUE,
    active BOOLEAN NOT NULL DEFAULT true
);

COMMENT ON COLUMN users.union_id IS 'the unique uuid for the user';
COMMENT ON COLUMN users.nickle_name IS 'the nickle name of the user';
COMMENT ON COLUMN users.username IS 'the username of the user';
COMMENT ON COLUMN users.password IS 'the entrypted password of the user';
COMMENT ON COLUMN users.email IS 'the email address of the user';
COMMENT ON COLUMN users.active IS 'is the user active';

CREATE TABLE IF NOT EXISTS groups (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name VARCHAR(32) NOT NULL UNIQUE,
    description VARCHAR(256)
);

COMMENT ON COLUMN groups.name IS 'the unique name of the group';
COMMENT ON COLUMN groups.description IS 'the optional description of the group';

CREATE TABLE IF NOT EXISTS namespaces (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name VARCHAR(32) NOT NULL UNIQUE,
    description VARCHAR(256)
);

COMMENT ON COLUMN namespaces.name IS 'the unique name of the namespace';
COMMENT ON COLUMN namespaces.description IS 'the optional description of the namespace';

CREATE TABLE IF NOT EXISTS capturers (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL,
    name VARCHAR(32) NOT NULL,
    version VARCHAR(32) NOT NULL
);

COMMENT ON COLUMN capturers.namespace_name IS 'the namespace capturer belongs to';
COMMENT ON COLUMN capturers.name IS 'the name of the capturer';
COMMENT ON COLUMN capturers.version IS 'the version of the capturer';

CREATE UNIQUE INDEX IF NOT EXISTS capturer_unique_index
    ON capturers (namespace_name, name, version);

CREATE TABLE IF NOT EXISTS access_keys (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    access_key_id VARCHAR(32) NOT NULL UNIQUE,
    access_key_secret VARCHAR(64) NOT NULL,
    description VARCHAR(256),
    expired_at TIMESTAMPTZ,
    last_access_at TIMESTAMPTZ
);

COMMENT ON COLUMN access_keys.enabled IS 'enable status of the access key';
COMMENT ON COLUMN access_keys.access_key_id IS 'the id of the access key';
COMMENT ON COLUMN access_keys.access_key_secret IS 'the secret key of the access key';
COMMENT ON COLUMN access_keys.description IS 'the optional description of the access key';
COMMENT ON COLUMN access_keys.expired_at IS 'time to live of the access key, nil means no expired';
COMMENT ON COLUMN access_keys.last_access_at IS 'the time any user last use this access key to access service';

CREATE TABLE IF NOT EXISTS rbac_policies (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ptype CHAR(1) NOT NULL,
    v0 VARCHAR(36) NOT NULL,
    v1 VARCHAR(512) NOT NULL,
    v2 VARCHAR(32) NOT NULL,
    v3 VARCHAR(64) NOT NULL DEFAULT ''
);

COMMENT ON COLUMN rbac_policies.ptype IS 'the type of the policy, only ''p''(policy) and ''g''(group)';
COMMENT ON COLUMN rbac_policies.v0 IS 'p(group name) OR g(user union id)';
COMMENT ON COLUMN rbac_policies.v1 IS 'p(target object name) OR g(group name)';
COMMENT ON COLUMN rbac_policies.v2 IS 'p(action name) OR g(namespace)';
COMMENT ON COLUMN rbac_policies.v3 IS 'p(effective ''allow'' or ''deny'') OR g(blank)';

CREATE UNIQUE INDEX IF NOT EXISTS rbac_policy_unique_index
    ON rbac_policies (ptype, v0, v1, v2);

CREATE TABLE IF NOT EXISTS rbac_policy_templates (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    target VARCHAR(512) NOT NULL,
    action VARCHAR(64) NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS unique_target_action
    ON rbac_policy_templates (target, action);

CREATE TABLE IF NOT EXISTS rule_tags (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    description VARCHAR(256) NOT NULL
);

COMMENT ON COLUMN rule_tags.enabled IS 'enable status of the tag';
COMMENT ON COLUMN rule_tags.description IS 'the optional description of the rule tag';

CREATE TABLE IF NOT EXISTS rule_conditions (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tag_id INT NOT NULL REFERENCES rule_tags(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    "window" BIGINT NOT NULL,
    threshold INT NOT NULL,
    expression VARCHAR(1024) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    description VARCHAR(256) NOT NULL
);

COMMENT ON COLUMN rule_conditions."window" IS 'the time window of the condition';
COMMENT ON COLUMN rule_conditions.threshold IS 'the threshold of the condition';
COMMENT ON COLUMN rule_conditions.expression IS 'the expression of the condition';
COMMENT ON COLUMN rule_conditions.enabled IS 'enable status of the condition';
COMMENT ON COLUMN rule_conditions.description IS 'the optional description of the rule condition';

-- +goose Down
DROP TABLE IF EXISTS rule_conditions;
DROP TABLE IF EXISTS rule_tags;
DROP TABLE IF EXISTS rbac_policy_templates;
DROP TABLE IF EXISTS rbac_policies;
DROP TABLE IF EXISTS access_keys;
DROP TABLE IF EXISTS capturers;
DROP TABLE IF EXISTS namespaces;
DROP TABLE IF EXISTS groups;
DROP TABLE IF EXISTS users;
