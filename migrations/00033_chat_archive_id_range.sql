-- +goose Up

ALTER TABLE chat_export_jobs
    ADD COLUMN from_message_id bigint NOT NULL DEFAULT 0,
    ADD COLUMN to_message_id   bigint NOT NULL DEFAULT 0;

ALTER TABLE chat_export_jobs
    ADD CONSTRAINT chk_chat_export_jobs_message_id_range
        CHECK (
            from_message_id >= 0
            AND to_message_id >= 0
            AND (from_message_id = 0 OR to_message_id = 0 OR from_message_id <= to_message_id)
        ),
    ADD CONSTRAINT chk_chat_export_jobs_range_mode
        CHECK (
            NOT (
                (from_date IS NOT NULL OR to_date IS NOT NULL)
                AND (from_message_id > 0 OR to_message_id > 0)
            )
        );

-- +goose Down
ALTER TABLE chat_export_jobs
    DROP CONSTRAINT IF EXISTS chk_chat_export_jobs_range_mode,
    DROP CONSTRAINT IF EXISTS chk_chat_export_jobs_message_id_range,
    DROP COLUMN IF EXISTS to_message_id,
    DROP COLUMN IF EXISTS from_message_id;
