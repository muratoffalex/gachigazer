-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_tasks_command_status_next ON tasks(command, status, next_attempt);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_tasks_command_status_next;
-- +goose StatementEnd
