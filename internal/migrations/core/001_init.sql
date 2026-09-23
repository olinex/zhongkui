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
    active BOOLEAN NOT NULL DEFAULT true,
    frontend_config TEXT NOT NULL DEFAULT '',
    frontend_config_hash CHAR(64) NOT NULL DEFAULT '',
    frontend_config_version BIGINT NOT NULL DEFAULT 0,
    frontend_config_sync_enabled BOOLEAN NOT NULL DEFAULT true
);

COMMENT ON COLUMN users.id IS 'the primary key of the user';
COMMENT ON COLUMN users.created_at IS 'the creation time of the user';
COMMENT ON COLUMN users.updated_at IS 'the last update time of the user';
COMMENT ON COLUMN users.deleted_at IS 'the soft delete time of the user, null means not deleted';
COMMENT ON COLUMN users.union_id IS 'the unique uuid for the user';
COMMENT ON COLUMN users.nickle_name IS 'the nickle name of the user';
COMMENT ON COLUMN users.username IS 'the username of the user';
COMMENT ON COLUMN users.password IS 'the encrypted password of the user';
COMMENT ON COLUMN users.email IS 'the email address of the user';
COMMENT ON COLUMN users.active IS 'is the user active';
COMMENT ON COLUMN users.frontend_config IS 'the user frontend page config, JSON text uploaded by the browser';
COMMENT ON COLUMN users.frontend_config_hash IS 'the sha256 hex digest of the frontend config, used for optimistic lock';
COMMENT ON COLUMN users.frontend_config_version IS 'the version of the frontend config, incremented on each successful sync';
COMMENT ON COLUMN users.frontend_config_sync_enabled IS 'whether the frontend config sync is enabled by the user';

CREATE TABLE IF NOT EXISTS user_password_resets (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email VARCHAR(128) NOT NULL,
    token_hash CHAR(64) NOT NULL UNIQUE,
    expired_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX idx_user_password_resets_user_id ON user_password_resets(user_id);

COMMENT ON COLUMN user_password_resets.id IS 'the primary key of the password reset record';
COMMENT ON COLUMN user_password_resets.created_at IS 'the creation time of the reset record';
COMMENT ON COLUMN user_password_resets.updated_at IS 'the last update time of the reset record';
COMMENT ON COLUMN user_password_resets.deleted_at IS 'the soft delete time of the reset record, null means not deleted';
COMMENT ON COLUMN user_password_resets.user_id IS 'the user requesting the password reset, cascade delete with the user';
COMMENT ON COLUMN user_password_resets.email IS 'the email requesting the password reset';
COMMENT ON COLUMN user_password_resets.token_hash IS 'the sha256 hex digest of the reset token, unique';
COMMENT ON COLUMN user_password_resets.expired_at IS 'the expiry time of the reset token';
COMMENT ON COLUMN user_password_resets.used IS 'whether the reset token has been used';

CREATE TABLE IF NOT EXISTS groups (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name VARCHAR(32) NOT NULL UNIQUE,
    description VARCHAR(256)
);

COMMENT ON COLUMN groups.id IS 'the primary key of the group';
COMMENT ON COLUMN groups.created_at IS 'the creation time of the group';
COMMENT ON COLUMN groups.updated_at IS 'the last update time of the group';
COMMENT ON COLUMN groups.name IS 'the unique name of the group';
COMMENT ON COLUMN groups.description IS 'the optional description of the group';

CREATE TABLE IF NOT EXISTS namespaces (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name VARCHAR(32) NOT NULL UNIQUE,
    description VARCHAR(256)
);

COMMENT ON COLUMN namespaces.id IS 'the primary key of the namespace';
COMMENT ON COLUMN namespaces.created_at IS 'the creation time of the namespace';
COMMENT ON COLUMN namespaces.updated_at IS 'the last update time of the namespace';
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

COMMENT ON COLUMN capturers.id IS 'the primary key of the capturer';
COMMENT ON COLUMN capturers.created_at IS 'the creation time of the capturer';
COMMENT ON COLUMN capturers.updated_at IS 'the last update time of the capturer';
COMMENT ON COLUMN capturers.namespace_name IS 'the namespace capturer belongs to';
COMMENT ON COLUMN capturers.name IS 'the name of the capturer';
COMMENT ON COLUMN capturers.version IS 'the version of the capturer';

CREATE UNIQUE INDEX IF NOT EXISTS uq_capturers_namespace_name_name_version
    ON capturers (namespace_name, name, version);

CREATE TABLE IF NOT EXISTS access_keys (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    access_key_id VARCHAR(32) NOT NULL UNIQUE,
    access_key_secret BYTEA NOT NULL,
    description VARCHAR(256),
    expired_at TIMESTAMPTZ,
    last_access_at TIMESTAMPTZ
);

COMMENT ON COLUMN access_keys.id IS 'the primary key of the access key';
COMMENT ON COLUMN access_keys.created_at IS 'the creation time of the access key';
COMMENT ON COLUMN access_keys.updated_at IS 'the last update time of the access key';
COMMENT ON COLUMN access_keys.user_id IS 'the owner user of the access key';
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
    v3 VARCHAR(64) NOT NULL DEFAULT '',
    v4 VARCHAR(64) NOT NULL DEFAULT '',
    v5 VARCHAR(64) NOT NULL DEFAULT ''
);

COMMENT ON COLUMN rbac_policies.id IS 'the primary key of the policy';
COMMENT ON COLUMN rbac_policies.created_at IS 'the creation time of the policy';
COMMENT ON COLUMN rbac_policies.updated_at IS 'the last update time of the policy';
COMMENT ON COLUMN rbac_policies.ptype IS 'the type of the policy, only ''p''(policy) and ''g''(group)';
COMMENT ON COLUMN rbac_policies.v0 IS 'p(group name) OR g(user union id)';
COMMENT ON COLUMN rbac_policies.v1 IS 'p(target object name) OR g(group name)';
COMMENT ON COLUMN rbac_policies.v2 IS 'p(action name) OR g(namespace)';
COMMENT ON COLUMN rbac_policies.v3 IS 'p(effective ''allow'' or ''deny'') OR g(blank)';
COMMENT ON COLUMN rbac_policies.v4 IS 'reserved column for gorm-adapter, blank';
COMMENT ON COLUMN rbac_policies.v5 IS 'reserved column for gorm-adapter, blank';

CREATE UNIQUE INDEX IF NOT EXISTS uq_rbac_policies_ptype_v0_v1_v2
    ON rbac_policies (ptype, v0, v1, v2);

CREATE TABLE IF NOT EXISTS rbac_policy_templates (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    target VARCHAR(512) NOT NULL,
    action VARCHAR(64) NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_rbac_policy_templates_target_action
    ON rbac_policy_templates (target, action);

COMMENT ON COLUMN rbac_policy_templates.id IS 'the primary key of the policy template';
COMMENT ON COLUMN rbac_policy_templates.created_at IS 'the creation time of the policy template';
COMMENT ON COLUMN rbac_policy_templates.updated_at IS 'the last update time of the policy template';
COMMENT ON COLUMN rbac_policy_templates.target IS 'the target object name of the policy template';
COMMENT ON COLUMN rbac_policy_templates.action IS 'the action name of the policy template';

CREATE TABLE IF NOT EXISTS notification_channels (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    type VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    config BYTEA,
    description VARCHAR(256) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_channels_namespace_name
    ON notification_channels (namespace_name, name);

CREATE INDEX IF NOT EXISTS idx_notification_channels_namespace
    ON notification_channels (namespace_name);

COMMENT ON COLUMN notification_channels.id IS 'the primary key of the notification channel';
COMMENT ON COLUMN notification_channels.created_at IS 'the creation time of the notification channel';
COMMENT ON COLUMN notification_channels.updated_at IS 'the last update time of the notification channel';
COMMENT ON COLUMN notification_channels.namespace_name IS 'the namespace the notification channel belongs to';
COMMENT ON COLUMN notification_channels.name IS 'the unique name of the notification channel within its namespace';
COMMENT ON COLUMN notification_channels.type IS 'the channel type key, e.g. email or webhook';
COMMENT ON COLUMN notification_channels.enabled IS 'is the notification channel enabled';
COMMENT ON COLUMN notification_channels.config IS 'the channel configuration (JSON text, may contain credentials), encrypted at rest as BYTEA';
COMMENT ON COLUMN notification_channels.description IS 'the optional description of the notification channel';

CREATE TABLE IF NOT EXISTS notification_targets (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    channel_id INT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    config BYTEA,
    language VARCHAR(32) NOT NULL DEFAULT '',
    description VARCHAR(256) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_targets_namespace_name
    ON notification_targets (namespace_name, name);

CREATE INDEX IF NOT EXISTS idx_notification_targets_namespace
    ON notification_targets (namespace_name);

CREATE INDEX IF NOT EXISTS idx_notification_targets_channel_id
    ON notification_targets (channel_id);

COMMENT ON COLUMN notification_targets.id IS 'the primary key of the notification target';
COMMENT ON COLUMN notification_targets.created_at IS 'the creation time of the notification target';
COMMENT ON COLUMN notification_targets.updated_at IS 'the last update time of the notification target';
COMMENT ON COLUMN notification_targets.namespace_name IS 'the namespace the notification target belongs to';
COMMENT ON COLUMN notification_targets.name IS 'the unique name of the notification target within its namespace';
COMMENT ON COLUMN notification_targets.channel_id IS 'the notification channel id the target belongs to';
COMMENT ON COLUMN notification_targets.type IS 'the snapshot of the channel type key, e.g. email or webhook';
COMMENT ON COLUMN notification_targets.enabled IS 'is the notification target enabled';
COMMENT ON COLUMN notification_targets.config IS 'the target configuration in JSON text, e.g. the recipient email or webhook url, encrypted at rest as BYTEA';
COMMENT ON COLUMN notification_targets.language IS 'the target-preferred message language, empty means the platform default';
COMMENT ON COLUMN notification_targets.description IS 'the optional description of the notification target';

CREATE TABLE IF NOT EXISTS notification_groups (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    description VARCHAR(256) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_groups_namespace_name
    ON notification_groups (namespace_name, name);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_groups_namespace_default
    ON notification_groups (namespace_name) WHERE is_default;

COMMENT ON COLUMN notification_groups.id IS 'the primary key of the notification group';
COMMENT ON COLUMN notification_groups.created_at IS 'the creation time of the notification group';
COMMENT ON COLUMN notification_groups.updated_at IS 'the last update time of the notification group';
COMMENT ON COLUMN notification_groups.namespace_name IS 'the namespace the notification group belongs to';
COMMENT ON COLUMN notification_groups.name IS 'the unique name of the notification group within its namespace';
COMMENT ON COLUMN notification_groups.is_default IS 'whether the group is the default group of the namespace, at most one default group per namespace';
COMMENT ON COLUMN notification_groups.description IS 'the optional description of the notification group';

CREATE TABLE IF NOT EXISTS notification_group_targets (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    group_id INT NOT NULL,
    target_id INT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_group_targets_group_target
    ON notification_group_targets (group_id, target_id);

CREATE INDEX IF NOT EXISTS idx_notification_group_targets_target_id
    ON notification_group_targets (target_id);

COMMENT ON COLUMN notification_group_targets.id IS 'the primary key of the group target association';
COMMENT ON COLUMN notification_group_targets.created_at IS 'the creation time of the group target association';
COMMENT ON COLUMN notification_group_targets.updated_at IS 'the last update time of the group target association';
COMMENT ON COLUMN notification_group_targets.group_id IS 'the notification group id of the association';
COMMENT ON COLUMN notification_group_targets.target_id IS 'the notification target id of the association';

CREATE TABLE IF NOT EXISTS notification_records (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    event_type VARCHAR(32) NOT NULL DEFAULT '',
    event_id BIGINT NOT NULL,
    group_id INT NOT NULL DEFAULT 0,
    target_id INT NOT NULL,
    channel_id INT NOT NULL DEFAULT 0,
    channel_type VARCHAR(32) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    message_id VARCHAR(128) NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '',
    notified_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_records_event_target
    ON notification_records (event_type, event_id, target_id);

CREATE INDEX IF NOT EXISTS idx_notification_records_namespace_created_at
    ON notification_records (namespace_name, created_at);

CREATE INDEX IF NOT EXISTS idx_notification_records_channel_id
    ON notification_records (channel_id);

COMMENT ON COLUMN notification_records.id IS 'the primary key of the notification record';
COMMENT ON COLUMN notification_records.created_at IS 'the creation time of the notification record';
COMMENT ON COLUMN notification_records.updated_at IS 'the last update time of the notification record';
COMMENT ON COLUMN notification_records.namespace_name IS 'the namespace the notification record belongs to';
COMMENT ON COLUMN notification_records.event_type IS 'the event type identifying what event the notification record''s event id refers to, e.g. tag event';
COMMENT ON COLUMN notification_records.event_id IS 'the tag event id that triggered the notification';
COMMENT ON COLUMN notification_records.group_id IS 'the notification group id at the sending time, 0 means not bound';
COMMENT ON COLUMN notification_records.target_id IS 'the notification target id, unique together with event_id';
COMMENT ON COLUMN notification_records.channel_id IS 'the channel id that sent the notification';
COMMENT ON COLUMN notification_records.channel_type IS 'the snapshot of the channel type key at the sending time';
COMMENT ON COLUMN notification_records.status IS 'the delivery status of the notification: pending/success/failed';
COMMENT ON COLUMN notification_records.error IS 'the error message when the delivery failed, empty on success';
COMMENT ON COLUMN notification_records.message_id IS 'the message id returned by the channel';
COMMENT ON COLUMN notification_records.payload IS 'the snapshot of the notification payload in JSON text';
COMMENT ON COLUMN notification_records.notified_at IS 'the actual delivery time of the notification, null means not delivered yet';

CREATE TABLE IF NOT EXISTS notification_templates (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    event VARCHAR(64) NOT NULL,
    channel_type VARCHAR(32) NOT NULL,
    language VARCHAR(32) NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    source VARCHAR(16) NOT NULL DEFAULT 'default',
    description VARCHAR(256) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_templates_namespace_event_channel_language
    ON notification_templates (namespace_name, event, channel_type, language);

CREATE INDEX IF NOT EXISTS idx_notification_templates_namespace
    ON notification_templates (namespace_name);

COMMENT ON COLUMN notification_templates.id IS 'the primary key of the notification template';
COMMENT ON COLUMN notification_templates.created_at IS 'the creation time of the notification template';
COMMENT ON COLUMN notification_templates.updated_at IS 'the last update time of the notification template';
COMMENT ON COLUMN notification_templates.deleted_at IS 'the soft delete time of the notification template, null means not deleted';
COMMENT ON COLUMN notification_templates.namespace_name IS 'the namespace the notification template belongs to';
COMMENT ON COLUMN notification_templates.event IS 'the event type key the template renders for, e.g. tag event record';
COMMENT ON COLUMN notification_templates.channel_type IS 'the channel type key the template applies to, * means all channels';
COMMENT ON COLUMN notification_templates.language IS 'the language the template is written in, * means all languages';
COMMENT ON COLUMN notification_templates.title IS 'the go text/template title template rendered against the event payload';
COMMENT ON COLUMN notification_templates.content IS 'the go text/template content template rendered against the event payload';
COMMENT ON COLUMN notification_templates.source IS 'the source of the template: default (built-in) or override (user-customized)';
COMMENT ON COLUMN notification_templates.description IS 'the optional description of the notification template';

CREATE TABLE IF NOT EXISTS tag_event_rules (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL,
    name VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    notification_group_id INT REFERENCES notification_groups(id) ON DELETE SET NULL,
    notification_target_id INT REFERENCES notification_targets(id) ON DELETE SET NULL,
    match_all BOOLEAN NOT NULL DEFAULT false,
    description VARCHAR(256) NOT NULL,
    suppress_window BIGINT NOT NULL DEFAULT 3600000000000
);

COMMENT ON COLUMN tag_event_rules.id IS 'the primary key of the tag event rule';
COMMENT ON COLUMN tag_event_rules.created_at IS 'the creation time of the tag event rule';
COMMENT ON COLUMN tag_event_rules.updated_at IS 'the last update time of the tag event rule';
COMMENT ON COLUMN tag_event_rules.namespace_name IS 'the namespace the tag event rule belongs to';
COMMENT ON COLUMN tag_event_rules.name IS 'the unique name of the tag event rule';
COMMENT ON COLUMN tag_event_rules.enabled IS 'enable status of the tag event rule';
COMMENT ON COLUMN tag_event_rules.notification_group_id IS 'the bound notification group id, null means not bound and the default group of the namespace is used';
COMMENT ON COLUMN tag_event_rules.notification_target_id IS 'the directly-bound notification target id, null means not bound; used to notify a single target without a group';
COMMENT ON COLUMN tag_event_rules.match_all IS 'whether all conditions must match to trigger the event; false (default) means any condition matches';
COMMENT ON COLUMN tag_event_rules.description IS 'the optional description of the tag event rule';

CREATE TABLE IF NOT EXISTS tag_event_conditions (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tag_event_rule_id INT NOT NULL REFERENCES tag_event_rules(id) ON DELETE CASCADE,
    model VARCHAR(255) NOT NULL DEFAULT '',
    dimension VARCHAR(128) NOT NULL DEFAULT '',
    item VARCHAR(255) NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL,
    "window" BIGINT NOT NULL,
    threshold INT NOT NULL,
    expression VARCHAR(1024) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    description VARCHAR(256) NOT NULL
);

COMMENT ON COLUMN tag_event_conditions.id IS 'the primary key of the tag event condition';
COMMENT ON COLUMN tag_event_conditions.created_at IS 'the creation time of the tag event condition';
COMMENT ON COLUMN tag_event_conditions.updated_at IS 'the last update time of the tag event condition';
COMMENT ON COLUMN tag_event_conditions.tag_event_rule_id IS 'the tag event rule this condition belongs to';
COMMENT ON COLUMN tag_event_conditions.model IS 'the target model type key filter of the condition, empty means no filter, e.g. zhongkui.com/zhongkui/internal/models/network.HttpTransport';
COMMENT ON COLUMN tag_event_conditions.dimension IS 'the tag dimension type filter of the condition, empty means no filter, e.g. fpTcpClientSig';
COMMENT ON COLUMN tag_event_conditions.item IS 'the concrete value filter of the tag dimension, empty means no filter, e.g. a fingerprint signature';
COMMENT ON COLUMN tag_event_conditions.name IS 'the unique name of the tag event condition';
COMMENT ON COLUMN tag_event_conditions."window" IS 'the time window of the condition';
COMMENT ON COLUMN tag_event_conditions.threshold IS 'the threshold of the condition';
COMMENT ON COLUMN tag_event_conditions.expression IS 'the boolean expression of tag names in the condition';
COMMENT ON COLUMN tag_event_conditions.enabled IS 'enable status of the condition';
COMMENT ON COLUMN tag_event_conditions.description IS 'the optional description of the tag event condition';

CREATE TABLE IF NOT EXISTS tag_event_rule_versions (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rule_id INT NOT NULL REFERENCES tag_event_rules(id) ON DELETE CASCADE,
    namespace_name VARCHAR(32) NOT NULL,
    version INT NOT NULL,
    name VARCHAR(255) NOT NULL,
    match_all BOOLEAN NOT NULL DEFAULT false,
    description VARCHAR(256) NOT NULL DEFAULT '',
    conditions TEXT NOT NULL,
    suppress_window BIGINT NOT NULL DEFAULT 3600000000000
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tag_event_rule_versions_rule_id_version
    ON tag_event_rule_versions (rule_id, version);

CREATE INDEX IF NOT EXISTS idx_tag_event_rule_versions_rule_id
    ON tag_event_rule_versions (rule_id);

COMMENT ON COLUMN tag_event_rule_versions.id IS 'the primary key of the tag event rule version';
COMMENT ON COLUMN tag_event_rule_versions.created_at IS 'the creation time of the tag event rule version';
COMMENT ON COLUMN tag_event_rule_versions.updated_at IS 'the last update time of the tag event rule version';
COMMENT ON COLUMN tag_event_rule_versions.rule_id IS 'the tag event rule this version belongs to';
COMMENT ON COLUMN tag_event_rule_versions.namespace_name IS 'the namespace of the tag event rule at the snapshot time';
COMMENT ON COLUMN tag_event_rule_versions.version IS 'the incremental version number of the tag event rule, starting from 1';
COMMENT ON COLUMN tag_event_rule_versions.name IS 'the name of the tag event rule at the snapshot time';
COMMENT ON COLUMN tag_event_rule_versions.match_all IS 'whether all conditions must match to trigger the event at the snapshot time; false (default) means any condition matches';
COMMENT ON COLUMN tag_event_rule_versions.description IS 'the description of the tag event rule at the snapshot time';
COMMENT ON COLUMN tag_event_rule_versions.conditions IS 'the JSON text snapshot of all condition business fields at the snapshot time';
COMMENT ON COLUMN tag_event_rule_versions.suppress_window IS 'the suppression window in nanoseconds of the rule at the snapshot time, 0 means no suppression';

CREATE TABLE IF NOT EXISTS tag_event_records (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL,
    rule_id INT NOT NULL REFERENCES tag_event_rules(id) ON DELETE CASCADE,
    rule_name VARCHAR(255) NOT NULL,
    match_all BOOLEAN NOT NULL DEFAULT false,
    dedup_key CHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL
);

COMMENT ON COLUMN tag_event_records.id IS 'the primary key of the tag event record';
COMMENT ON COLUMN tag_event_records.created_at IS 'the creation time of the tag event record';
COMMENT ON COLUMN tag_event_records.updated_at IS 'the last update time of the tag event record';
COMMENT ON COLUMN tag_event_records.namespace_name IS 'the namespace the tag event record belongs to';
COMMENT ON COLUMN tag_event_records.rule_id IS 'the tag event rule that triggered the tag event record';
COMMENT ON COLUMN tag_event_records.rule_name IS 'the name of the tag event rule at the trigger time';
COMMENT ON COLUMN tag_event_records.match_all IS 'whether all conditions must match to trigger the event';
COMMENT ON COLUMN tag_event_records.dedup_key IS 'the sha256 hex suppression fingerprint of the event, namespace + rule id + sorted hit group keys';
COMMENT ON COLUMN tag_event_records.status IS 'the status of the tag event record: pending/alerting/suppressed/closed';
COMMENT ON COLUMN tag_event_records.detected_at IS 'the detection time of the tag event record';

CREATE INDEX IF NOT EXISTS idx_tag_event_records_dedup_key_created_at
    ON tag_event_records (dedup_key, created_at);

CREATE INDEX IF NOT EXISTS idx_tag_event_records_rule_id
    ON tag_event_records (rule_id);

CREATE INDEX IF NOT EXISTS idx_tag_event_records_status_created_at
    ON tag_event_records (status, created_at);

CREATE TABLE IF NOT EXISTS tag_event_record_hits (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tag_event_record_id BIGINT NOT NULL REFERENCES tag_event_records(id) ON DELETE CASCADE,
    condition_id INT NOT NULL,
    condition_name VARCHAR(255) NOT NULL,
    model VARCHAR(255) NOT NULL DEFAULT '',
    dimension VARCHAR(128) NOT NULL DEFAULT '',
    item VARCHAR(255) NOT NULL DEFAULT '',
    count BIGINT NOT NULL,
    threshold INT NOT NULL,
    "window" BIGINT NOT NULL,
    ids JSONB DEFAULT '[]'
);

COMMENT ON COLUMN tag_event_record_hits.id IS 'the primary key of the tag event record hit';
COMMENT ON COLUMN tag_event_record_hits.created_at IS 'the creation time of the tag event record hit';
COMMENT ON COLUMN tag_event_record_hits.updated_at IS 'the last update time of the tag event record hit';
COMMENT ON COLUMN tag_event_record_hits.tag_event_record_id IS 'the tag event record this hit belongs to';
COMMENT ON COLUMN tag_event_record_hits.condition_id IS 'the id of the tag event condition that produced the hit';
COMMENT ON COLUMN tag_event_record_hits.condition_name IS 'the name of the tag event condition at the trigger time';
COMMENT ON COLUMN tag_event_record_hits.model IS 'the target model type key filter of the condition, empty means no filter';
COMMENT ON COLUMN tag_event_record_hits.dimension IS 'the tag dimension type filter of the condition, empty means no filter';
COMMENT ON COLUMN tag_event_record_hits.item IS 'the concrete value filter of the tag dimension, empty means no filter';
COMMENT ON COLUMN tag_event_record_hits.count IS 'the hit count of the condition within the window';
COMMENT ON COLUMN tag_event_record_hits.threshold IS 'the threshold of the condition';
COMMENT ON COLUMN tag_event_record_hits."window" IS 'the time window of the condition in nanoseconds';
COMMENT ON COLUMN tag_event_record_hits.ids IS 'the JSONB array of the original record ids of the hit group, aggregated and deduplicated by the detection query';

CREATE INDEX IF NOT EXISTS idx_tag_event_record_hits_tag_event_record_id
    ON tag_event_record_hits (tag_event_record_id);

CREATE TABLE IF NOT EXISTS tag_rules (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL,
    name VARCHAR(128) NOT NULL,
    description VARCHAR(512) NOT NULL DEFAULT '',
    model VARCHAR(255) NOT NULL,
    rego TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    CONSTRAINT uq_tag_rules_namespace_name_model_name UNIQUE (namespace_name, model, name)
);

COMMENT ON COLUMN tag_rules.id IS 'the primary key of the tag rule';
COMMENT ON COLUMN tag_rules.created_at IS 'the creation time of the tag rule';
COMMENT ON COLUMN tag_rules.updated_at IS 'the last update time of the tag rule';
COMMENT ON COLUMN tag_rules.namespace_name IS 'the namespace the tag rule belongs to';
COMMENT ON COLUMN tag_rules.name IS 'the unique name of the rule within its group';
COMMENT ON COLUMN tag_rules.description IS 'the description of the rule';
COMMENT ON COLUMN tag_rules.model IS 'the target model type key, e.g. zhongkui.com/zhongkui/internal/models/network.HttpTransport';
COMMENT ON COLUMN tag_rules.rego IS 'the rego rule source, contract: package zk producing tags set';
COMMENT ON COLUMN tag_rules.enabled IS 'is the rule enabled';

CREATE TABLE IF NOT EXISTS tag_rule_versions (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rule_id INT NOT NULL REFERENCES tag_rules(id) ON DELETE CASCADE,
    namespace_name VARCHAR(32) NOT NULL,
    version INT NOT NULL,
    name VARCHAR(128) NOT NULL,
    description VARCHAR(512) NOT NULL DEFAULT '',
    model VARCHAR(255) NOT NULL,
    rego TEXT NOT NULL,
    CONSTRAINT uq_tag_rule_versions_rule_id_version UNIQUE (rule_id, version)
);

CREATE INDEX IF NOT EXISTS idx_tag_rule_versions_rule_id
    ON tag_rule_versions (rule_id);

COMMENT ON COLUMN tag_rule_versions.id IS 'the primary key of the tag rule version';
COMMENT ON COLUMN tag_rule_versions.created_at IS 'the creation time of the tag rule version';
COMMENT ON COLUMN tag_rule_versions.updated_at IS 'the last update time of the tag rule version';
COMMENT ON COLUMN tag_rule_versions.rule_id IS 'the tag rule this version belongs to';
COMMENT ON COLUMN tag_rule_versions.namespace_name IS 'the namespace of the tag rule at the snapshot time';
COMMENT ON COLUMN tag_rule_versions.version IS 'the incremental version number of the rule, starting from 1';
COMMENT ON COLUMN tag_rule_versions.name IS 'the name of the rule at the snapshot time';
COMMENT ON COLUMN tag_rule_versions.description IS 'the description of the rule at the snapshot time';
COMMENT ON COLUMN tag_rule_versions.model IS 'the target model type key at the snapshot time';
COMMENT ON COLUMN tag_rule_versions.rego IS 'the rego rule source at the snapshot time';

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

CREATE TABLE IF NOT EXISTS datasources (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    type VARCHAR(16) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    config BYTEA,
    description VARCHAR(256) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_datasources_namespace_name ON datasources (namespace_name, name);
CREATE INDEX IF NOT EXISTS idx_datasources_namespace ON datasources (namespace_name);

COMMENT ON COLUMN datasources.id IS 'the primary key of the datasource';
COMMENT ON COLUMN datasources.created_at IS 'the creation time of the datasource';
COMMENT ON COLUMN datasources.updated_at IS 'the last update time of the datasource';
COMMENT ON COLUMN datasources.namespace_name IS 'the namespace the datasource belongs to';
COMMENT ON COLUMN datasources.name IS 'the unique name of the datasource within its namespace, used as the bare-name vtable';
COMMENT ON COLUMN datasources.type IS 'the datasource type key, e.g. redis or mysql';
COMMENT ON COLUMN datasources.enabled IS 'is the datasource enabled';
COMMENT ON COLUMN datasources.config IS 'the connection config (JSON text, may contain credentials), encrypted at rest as BYTEA';
COMMENT ON COLUMN datasources.description IS 'the optional description of the datasource';

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS knowledge_documents (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    title VARCHAR(256) NOT NULL,
    content TEXT NOT NULL,
    chunk_count INT NOT NULL DEFAULT 0,
    hash CHAR(64) NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_knowledge_documents_namespace
    ON knowledge_documents (namespace_name);

COMMENT ON COLUMN knowledge_documents.id IS 'the primary key of the knowledge document';
COMMENT ON COLUMN knowledge_documents.created_at IS 'the creation time of the knowledge document';
COMMENT ON COLUMN knowledge_documents.updated_at IS 'the last update time of the knowledge document';
COMMENT ON COLUMN knowledge_documents.deleted_at IS 'the soft delete time of the knowledge document, null means not deleted';
COMMENT ON COLUMN knowledge_documents.namespace_name IS 'the namespace the knowledge document belongs to';
COMMENT ON COLUMN knowledge_documents.title IS 'the title of the knowledge document';
COMMENT ON COLUMN knowledge_documents.content IS 'the full content of the knowledge document';
COMMENT ON COLUMN knowledge_documents.chunk_count IS 'the number of chunks the document is split into';
COMMENT ON COLUMN knowledge_documents.hash IS 'the sha256 hex digest of the document content, used to detect stale chunk indexes';

CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    document_id INT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    chunk_index INT NOT NULL,
    heading_path TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    embedding vector(1536) NOT NULL,
    search tsvector NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_namespace_document
    ON knowledge_chunks (namespace_name, document_id);

CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_embedding
    ON knowledge_chunks USING hnsw (embedding vector_cosine_ops);

CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_search
    ON knowledge_chunks USING gin (search);

COMMENT ON COLUMN knowledge_chunks.id IS 'the primary key of the knowledge chunk';
COMMENT ON COLUMN knowledge_chunks.created_at IS 'the creation time of the knowledge chunk';
COMMENT ON COLUMN knowledge_chunks.updated_at IS 'the last update time of the knowledge chunk';
COMMENT ON COLUMN knowledge_chunks.document_id IS 'the owning knowledge document id, cascade delete with the document';
COMMENT ON COLUMN knowledge_chunks.namespace_name IS 'the namespace the knowledge chunk belongs to';
COMMENT ON COLUMN knowledge_chunks.chunk_index IS 'the index of the chunk within the document, starting from 0';
COMMENT ON COLUMN knowledge_chunks.heading_path IS 'the heading path of the chunk within the document, used for citation traceability';
COMMENT ON COLUMN knowledge_chunks.content IS 'the content of the chunk';
COMMENT ON COLUMN knowledge_chunks.embedding IS 'the vectorized representation of the chunk content for similarity retrieval';
COMMENT ON COLUMN knowledge_chunks.search IS 'the full-text search representation of the chunk content';

CREATE TABLE IF NOT EXISTS agent_sessions (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    title VARCHAR(256) NOT NULL DEFAULT '',
    target_model VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active'
);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_namespace
    ON agent_sessions (namespace_name);

COMMENT ON COLUMN agent_sessions.id IS 'the primary key of the agent session';
COMMENT ON COLUMN agent_sessions.created_at IS 'the creation time of the agent session';
COMMENT ON COLUMN agent_sessions.updated_at IS 'the last update time of the agent session';
COMMENT ON COLUMN agent_sessions.deleted_at IS 'the soft delete time of the agent session, null means not deleted';
COMMENT ON COLUMN agent_sessions.namespace_name IS 'the namespace the agent session belongs to';
COMMENT ON COLUMN agent_sessions.title IS 'the title of the agent session, defaulting to a summary of the first user message';
COMMENT ON COLUMN agent_sessions.target_model IS 'the default slot of the session (flash/standard/advanced), empty means resolving through the agent.defaultSlot parameter';
COMMENT ON COLUMN agent_sessions.status IS 'the status of the agent session, default active';

CREATE TABLE IF NOT EXISTS agent_messages (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_id BIGINT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
    namespace_name VARCHAR(32) NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    draft_json JSONB,
    citations_json JSONB
);

CREATE INDEX IF NOT EXISTS idx_agent_messages_session_id
    ON agent_messages (session_id);

COMMENT ON COLUMN agent_messages.id IS 'the primary key of the agent message';
COMMENT ON COLUMN agent_messages.created_at IS 'the creation time of the agent message';
COMMENT ON COLUMN agent_messages.updated_at IS 'the last update time of the agent message';
COMMENT ON COLUMN agent_messages.session_id IS 'the owning agent session id, cascade delete with the session';
COMMENT ON COLUMN agent_messages.namespace_name IS 'the namespace the agent message belongs to';
COMMENT ON COLUMN agent_messages.role IS 'the role of the message, e.g. user, assistant or system';
COMMENT ON COLUMN agent_messages.content IS 'the content of the message';
COMMENT ON COLUMN agent_messages.draft_json IS 'the draft payload of the assistant reply, e.g. intermediate tool-call states, null when not applicable';
COMMENT ON COLUMN agent_messages.citations_json IS 'the citation source list of the assistant reply, null when not applicable';

CREATE TABLE IF NOT EXISTS llm_models (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    name VARCHAR(64) NOT NULL,
    kind VARCHAR(16) NOT NULL,
    base_url VARCHAR(512) NOT NULL DEFAULT '',
    api_key BYTEA,
    model VARCHAR(128) NOT NULL DEFAULT '',
    timeout_seconds INT NOT NULL DEFAULT 0,
    temperature DOUBLE PRECISION NOT NULL DEFAULT 0,
    max_tokens INT NOT NULL DEFAULT 0,
    dimensions INT NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT true,
    description VARCHAR(512) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_llm_models_name ON llm_models (name);
CREATE INDEX IF NOT EXISTS idx_llm_models_kind ON llm_models (kind);

COMMENT ON COLUMN llm_models.id IS 'the primary key of the llm model';
COMMENT ON COLUMN llm_models.created_at IS 'the creation time of the llm model';
COMMENT ON COLUMN llm_models.updated_at IS 'the last update time of the llm model';
COMMENT ON COLUMN llm_models.deleted_at IS 'the soft delete time of the llm model, null means not deleted';
COMMENT ON COLUMN llm_models.name IS 'the globally unique local name of the llm model';
COMMENT ON COLUMN llm_models.kind IS 'the kind of the llm model, chat or embedding';
COMMENT ON COLUMN llm_models.base_url IS 'the base address of the model endpoint (OpenAI compatible API)';
COMMENT ON COLUMN llm_models.api_key IS 'the credential of the model endpoint, encrypted at rest as BYTEA';
COMMENT ON COLUMN llm_models.model IS 'the vendor-side model name, e.g. gpt-4o-mini';
COMMENT ON COLUMN llm_models.timeout_seconds IS 'the timeout in seconds of a single request, 0 means the caller default';
COMMENT ON COLUMN llm_models.temperature IS 'the sampling temperature within [0, 2], only meaningful for chat models';
COMMENT ON COLUMN llm_models.max_tokens IS 'the maximum number of tokens per generation, only meaningful for chat models';
COMMENT ON COLUMN llm_models.dimensions IS 'the vector dimensions, only meaningful for embedding models';
COMMENT ON COLUMN llm_models.enabled IS 'is the llm model enabled, disabled models cannot be bound to a slot';
COMMENT ON COLUMN llm_models.description IS 'the optional description of the llm model';

CREATE TABLE IF NOT EXISTS llm_bindings (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    slot VARCHAR(16) NOT NULL,
    model_id BIGINT NOT NULL REFERENCES llm_models(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_llm_bindings_slot ON llm_bindings (slot);
CREATE INDEX IF NOT EXISTS idx_llm_bindings_model_id ON llm_bindings (model_id);

COMMENT ON COLUMN llm_bindings.id IS 'the primary key of the llm slot binding';
COMMENT ON COLUMN llm_bindings.created_at IS 'the creation time of the llm slot binding';
COMMENT ON COLUMN llm_bindings.updated_at IS 'the last update time of the llm slot binding';
COMMENT ON COLUMN llm_bindings.slot IS 'the bound slot, one of flash, standard, advanced or embedding, unique';
COMMENT ON COLUMN llm_bindings.model_id IS 'the bound llm model id, deletion of a bound model is restricted';

CREATE TABLE IF NOT EXISTS agent_params (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    key VARCHAR(128) NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    default_value TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_params_key ON agent_params (key);

COMMENT ON COLUMN agent_params.id IS 'the primary key of the agent parameter';
COMMENT ON COLUMN agent_params.created_at IS 'the creation time of the agent parameter';
COMMENT ON COLUMN agent_params.updated_at IS 'the last update time of the agent parameter';
COMMENT ON COLUMN agent_params.key IS 'the parameter key, dot-separated, e.g. agent.systemPrompt, declared by the in-code parameter registry';
COMMENT ON COLUMN agent_params.value IS 'the effective value of the parameter, interpreted by the kind declared in the in-code registry';
COMMENT ON COLUMN agent_params.default_value IS 'the factory value of the parameter, seeded by the migrate fixtures and never written through any API path';

-- +goose Down
DROP TABLE IF EXISTS agent_params;
DROP TABLE IF EXISTS llm_bindings;
DROP TABLE IF EXISTS llm_models;
DROP TABLE IF EXISTS agent_messages;
DROP TABLE IF EXISTS agent_sessions;
DROP TABLE IF EXISTS knowledge_chunks;
DROP TABLE IF EXISTS knowledge_documents;
DROP TABLE IF EXISTS tag_event_record_hits;
DROP TABLE IF EXISTS tag_event_records;
DROP TABLE IF EXISTS tag_rule_versions;
DROP TABLE IF EXISTS tag_rules;
DROP TABLE IF EXISTS tag_rule_samples;
DROP TABLE IF EXISTS tag_event_conditions;
DROP TABLE IF EXISTS tag_event_rule_versions;
DROP TABLE IF EXISTS tag_event_rules;
DROP TABLE IF EXISTS notification_group_targets;
DROP TABLE IF EXISTS notification_targets;
DROP TABLE IF EXISTS notification_groups;
DROP TABLE IF EXISTS notification_records;
DROP TABLE IF EXISTS notification_templates;
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS rbac_policy_templates;
DROP TABLE IF EXISTS rbac_policies;
DROP TABLE IF EXISTS access_keys;
DROP TABLE IF EXISTS user_password_resets;
DROP TABLE IF EXISTS capturers;
DROP TABLE IF EXISTS datasources;
DROP TABLE IF EXISTS namespaces;
DROP TABLE IF EXISTS groups;
DROP TABLE IF EXISTS users;
