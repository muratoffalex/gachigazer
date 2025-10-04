-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS messages (
    chat_id INTEGER NOT NULL,
    message_id INTEGER NOT NULL,
    media_group_id TEXT,
    main INTEGER DEFAULT 1 NOT NULL,
    username TEXT NOT NULL,
    data BLOB NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (chat_id, message_id)
);
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    public_id TEXT UNIQUE,
    username TEXT,
    first_name TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command TEXT NOT NULL,
    update_data TEXT NOT NULL,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER NOT NULL,
    retry_delay INTEGER NOT NULL,
    last_attempt DATETIME,
    next_attempt DATETIME,
    status TEXT DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS cache (
    key TEXT PRIMARY KEY,
    data BLOB NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS conversation_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_message_id INTEGER REFERENCES conversation_history(id),
    conversation_chain_id TEXT NOT NULL DEFAULT '',
    tool_calls TEXT,
    tool_responses TEXT,
    chat_id INTEGER NOT NULL,
    message_id INTEGER NOT NULL,
    reply_to_message_id INTEGER,
    user_id INTEGER NOT NULL,
    role TEXT NOT NULL, -- 'user' or 'assistant'
    text TEXT NOT NULL,
    model_name TEXT,
    total_tokens INTEGER,
    completion_tokens INTEGER,
    prompt_tokens INTEGER,
    total_cost REAL,
    attempts_count INTEGER NOT NULL DEFAULT 1,
    params TEXT NOT NULL DEFAULT '{}',
    images TEXT NOT NULL DEFAULT '[]',
    audio TEXT NOT NULL DEFAULT '[]',
    files TEXT NOT NULL DEFAULT '[]',
    urls TEXT NOT NULL DEFAULT '[]',
    annotations TEXT,
    conversation_id INTEGER NOT NULL DEFAULT 0,
    conversation_title TEXT,
    conversation_summary TEXT,
    tool_name TEXT,
    tool_params TEXT,
    conversation_title_source TEXT,
    is_first BOOLEAN NOT NULL DEFAULT FALSE,
    saved BOOLEAN NOT NULL DEFAULT FALSE,
    saved_at DATETIME,
    saved_by INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS chat_settings (
    chat_id INTEGER PRIMARY KEY,
    model TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chain_id ON conversation_history(conversation_chain_id);
CREATE INDEX IF NOT EXISTS idx_parent_msg ON conversation_history(parent_message_id);
CREATE INDEX IF NOT EXISTS idx_conversation_history_chat_message ON conversation_history(chat_id, message_id);
CREATE INDEX IF NOT EXISTS idx_conversation_history_chat_message_id ON conversation_history(chat_id, message_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_messages_chat_message ON messages(chat_id, message_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_messages_chat_message;
DROP INDEX IF EXISTS idx_conversation_history_chat_message_id;
DROP INDEX IF EXISTS idx_conversation_history_chat_message;
DROP INDEX IF EXISTS idx_parent_msg;
DROP INDEX IF EXISTS idx_chain_id;

DROP TABLE IF EXISTS chat_settings;
DROP TABLE IF EXISTS conversation_history;
DROP TABLE IF EXISTS cache;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS messages;
-- +goose StatementEnd
