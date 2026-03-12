-- Team workspace: shared file storage scoped by (team, channel, chat_id).
CREATE TABLE team_workspace_files (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    team_id     UUID NOT NULL REFERENCES agent_teams(id) ON DELETE CASCADE,
    channel     VARCHAR(50)  NOT NULL DEFAULT '',
    chat_id     VARCHAR(255) NOT NULL DEFAULT '',
    file_name   VARCHAR(255) NOT NULL,
    mime_type   VARCHAR(100),
    file_path   TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    uploaded_by UUID NOT NULL REFERENCES agents(id),
    task_id     UUID REFERENCES team_tasks(id) ON DELETE SET NULL,
    pinned      BOOLEAN NOT NULL DEFAULT false,
    tags        TEXT[] NOT NULL DEFAULT '{}',
    metadata    JSONB DEFAULT '{}',
    archived_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(team_id, channel, chat_id, file_name)
);

CREATE INDEX idx_twf_team_channel ON team_workspace_files(team_id, channel, chat_id);
CREATE INDEX idx_twf_uploaded_by  ON team_workspace_files(uploaded_by);
CREATE INDEX idx_twf_task         ON team_workspace_files(task_id) WHERE task_id IS NOT NULL;
CREATE INDEX idx_twf_archived     ON team_workspace_files(archived_at) WHERE archived_at IS NOT NULL;
CREATE INDEX idx_twf_pinned       ON team_workspace_files(team_id, pinned) WHERE pinned = true;
CREATE INDEX idx_twf_tags         ON team_workspace_files USING GIN(tags);

-- File version history.
CREATE TABLE team_workspace_file_versions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    file_id     UUID NOT NULL REFERENCES team_workspace_files(id) ON DELETE CASCADE,
    version     INT NOT NULL,
    file_path   TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    uploaded_by UUID NOT NULL REFERENCES agents(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(file_id, version)
);

CREATE INDEX idx_twfv_file ON team_workspace_file_versions(file_id);

-- File comments / annotations.
CREATE TABLE team_workspace_comments (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    file_id     UUID NOT NULL REFERENCES team_workspace_files(id) ON DELETE CASCADE,
    agent_id    UUID NOT NULL REFERENCES agents(id),
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_twfc_file ON team_workspace_comments(file_id);
