-- +goose Up
ALTER TABLE filters ADD COLUMN revision integer NOT NULL DEFAULT 1;
CREATE TABLE filter_versions (
    filter_id bigint NOT NULL REFERENCES filters(id) ON DELETE CASCADE,
    revision integer NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    conditions jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (filter_id, revision)
);
INSERT INTO filter_versions (filter_id, revision, name, description, conditions)
SELECT id, revision, name, COALESCE(description, ''), conditions FROM filters;

CREATE TABLE message_decisions (
    id bigserial PRIMARY KEY,
    message_id bigint NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    source_id bigint NOT NULL,
    message_revision integer NOT NULL,
    trigger text NOT NULL,
    status text NOT NULL,
    filter_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    evaluation jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_message_decisions_message ON message_decisions(message_id, id DESC);
CREATE INDEX idx_message_decisions_source ON message_decisions(source_id, id DESC);
CREATE INDEX idx_message_decisions_status ON message_decisions(status, id DESC);
CREATE INDEX idx_message_decisions_filters ON message_decisions USING gin(filter_ids);
CREATE INDEX idx_message_decisions_created ON message_decisions(created_at);
ALTER TABLE ai_digest_run_items ADD COLUMN filter_trace jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE ai_digest_run_items DROP COLUMN filter_trace;
DROP TABLE message_decisions;
DROP TABLE filter_versions;
ALTER TABLE filters DROP COLUMN revision;
