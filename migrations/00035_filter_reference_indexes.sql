-- +goose Up
CREATE INDEX idx_flow_nodes_shared_filter_ids
    ON flow_nodes USING gin ((COALESCE(config->'filter_ids', '[]'::jsonb)))
    WHERE type = 'filter';
CREATE INDEX idx_ai_digest_profiles_shared_filter_ids
    ON ai_digest_profiles USING gin ((COALESCE(filter_ids, '[]'::jsonb)));
CREATE INDEX idx_ai_digest_profiles_legacy_filter_id ON ai_digest_profiles(filter_id);

-- +goose Down
DROP INDEX idx_ai_digest_profiles_legacy_filter_id;
DROP INDEX idx_ai_digest_profiles_shared_filter_ids;
DROP INDEX idx_flow_nodes_shared_filter_ids;
