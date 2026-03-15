-- API key management: multiple keys with fine-grained scopes
CREATE TABLE api_keys (
    id            UUID PRIMARY KEY,
    name          VARCHAR(100) NOT NULL,
    prefix        VARCHAR(8)   NOT NULL,              -- first 8 chars for display identification
    key_hash      VARCHAR(64)  NOT NULL UNIQUE,       -- SHA-256 hex digest
    scopes        TEXT[]       NOT NULL DEFAULT '{}',  -- e.g. {'operator.admin','operator.read'}
    expires_at    TIMESTAMPTZ,                         -- NULL = never expires
    last_used_at  TIMESTAMPTZ,
    revoked       BOOLEAN      NOT NULL DEFAULT false,
    created_by    VARCHAR(255),                        -- user ID who created the key
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Fast lookup by hash (only active keys)
CREATE INDEX idx_api_keys_key_hash ON api_keys (key_hash) WHERE NOT revoked;

-- Fast lookup by prefix (for display/search)
CREATE INDEX idx_api_keys_prefix ON api_keys (prefix);
