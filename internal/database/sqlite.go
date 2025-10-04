package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/telegram"
	_ "modernc.org/sqlite"
)

type sqliteDB struct {
	db     *sql.DB
	logger logger.Logger
}

func NewSQLiteDB(cfg *config.Config, log logger.Logger) (Database, error) {
	db, err := sql.Open("sqlite", cfg.GetDatabaseDSN())
	if err != nil {
		return nil, err
	}

	log.WithFields(logger.Fields{
		"DSN": cfg.GetDatabaseDSN(),
	}).Debug("Database opened")

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.WithFields(logger.Fields{
		"DSN": cfg.GetDatabaseDSN(),
	}).Debug("Database alive")

	if err := RunMigrations(db); err != nil {
		return nil, err
	}

	return &sqliteDB{db: db, logger: log}, nil
}

func (s *sqliteDB) Exec(query string, args ...any) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

func (s *sqliteDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

func (s *sqliteDB) Query(query string, args ...any) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}

func (s *sqliteDB) QueryRow(query string, args ...any) *sql.Row {
	return s.db.QueryRow(query, args...)
}

func (s *sqliteDB) Close() error {
	return s.db.Close()
}

func (s *sqliteDB) ExecWithRetry(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var res sql.Result
	var err error
	for i := range 3 {
		res, err = s.ExecContext(ctx, query, args...)
		if err == nil || !strings.Contains(err.Error(), "database is locked") {
			return res, err
		}
		s.logger.WithFields(logger.Fields{
			"attempt": i + 1,
			"query":   query,
			"error":   err.Error(),
		}).Warn("Database locked, retrying...")
		time.Sleep(100 * time.Millisecond * time.Duration(i+1))
	}
	return res, err
}

func (s *sqliteDB) SaveChatModel(chatID int64, model string) error {
	_, err := s.db.Exec(`
		INSERT INTO chat_settings (chat_id, model) 
		VALUES (?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET model = excluded.model, updated_at = CURRENT_TIMESTAMP
	`, chatID, model)
	return err
}

func (s *sqliteDB) GetChatModel(chatID int64) (string, error) {
	var model string
	err := s.db.QueryRow("SELECT model FROM chat_settings WHERE chat_id = ?", chatID).Scan(&model)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return model, err
}

func (s *sqliteDB) DeleteChatModel(chatID int64) error {
	_, err := s.db.Exec("DELETE FROM chat_settings WHERE chat_id = ?", chatID)
	return err
}

func (s *sqliteDB) SaveMessage(chatID int64, messageID int, username, mediaGroupID string, main bool, data []byte) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO messages (chat_id, message_id, media_group_id, main, data, username) 
		VALUES (?, ?, ?, ?, ?, ?)
	`, chatID, messageID, mediaGroupID, main, data, username)
	return err
}

func (s *sqliteDB) UpdateMessage(chatID int64, messageID int, data []byte) error {
	_, err := s.db.Exec(`
		UPDATE messages set data = ? where chat_id = ? and message_id = ?
	`, data, chatID, messageID)
	return err
}

func (s *sqliteDB) GetMessagesWithMediaGroupID(chatID int64, mediaGroupID string) ([]telegram.Update, error) {
	var updates []telegram.Update
	rows, err := s.db.Query("SELECT data FROM messages WHERE chat_id = ? AND media_group_id = ?", chatID, mediaGroupID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("failed to scan message data: %w", err)
		}

		var update telegram.Update
		if err := json.Unmarshal(data, &update); err != nil {
			log.Printf("Failed to unmarshal update: %v", err)
			continue // skip incorrect
		}

		updates = append(updates, update)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return updates, nil
}

func (s *sqliteDB) GetMessage(chatID int64, messageID int) (*telegram.Update, error) {
	var update *telegram.Update
	var data []byte

	err := s.db.QueryRow("SELECT data FROM messages WHERE chat_id = ? AND message_id = ?", chatID, messageID).Scan(&data)
	if err != nil {
		return update, fmt.Errorf("failed to get message: %w", err)
	}

	if err := json.Unmarshal(data, &update); err != nil {
		return update, fmt.Errorf("failed to unmarshal update: %w", err)
	}

	return update, nil
}

func (s *sqliteDB) GetMessagesBy(chatID int64, ignoreMessageID int64, duration time.Duration, count int, username string) ([]telegram.Update, error) {
	var updates []telegram.Update

	baseQuery := `SELECT data`

	if count > 0 {
		baseQuery += `, created_at`
	}

	baseQuery += ` FROM messages WHERE chat_id = ? and message_id != ? and main = 1`
	args := []any{chatID, ignoreMessageID}

	// Add conditions for filtering
	if duration > 0 {
		baseQuery += " AND created_at >= datetime('now', ?)"
		args = append(args, fmt.Sprintf("-%d seconds", int(duration.Seconds())))
	}

	if username != "" {
		baseQuery += " AND username LIKE ? || '%'"
		args = append(args, username)
	}

	baseQuery += " ORDER BY created_at"

	var query string
	if count > 0 {
		baseQuery += " DESC LIMIT ?"
		query = fmt.Sprintf(`
            SELECT data FROM (
                %s
            ) ORDER BY created_at ASC`, baseQuery)
		args = append(args, count)
	} else {
		query = baseQuery
	}

	log.Printf("Executing query: %s with args: %v", query, args)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("failed to scan message data: %w", err)
		}

		var update telegram.Update
		if err := json.Unmarshal(data, &update); err != nil {
			log.Printf("Failed to unmarshal update: %v", err)
			continue // Skip invalid records
		}

		updates = append(updates, update)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return updates, nil
}

func (s *sqliteDB) PurgeOldMessages(retentionDays int) error {
	_, err := s.db.Exec("DELETE FROM messages WHERE created_at < datetime('now', ?)", fmt.Sprintf("-%d days", retentionDays))
	return err
}

func (s *sqliteDB) LoadAllChatModels() (map[int64]string, error) {
	models := make(map[int64]string)
	rows, err := s.db.Query("SELECT chat_id, model FROM chat_settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var chatID int64
		var model string
		if err := rows.Scan(&chatID, &model); err != nil {
			return nil, err
		}
		models[chatID] = model
	}

	return models, nil
}

func (s *sqliteDB) GetDB() *sql.DB {
	return s.db
}
