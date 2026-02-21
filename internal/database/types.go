package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/muratoffalex/gachigazer/internal/telegram"
)

type Database interface {
	GetDB() *sql.DB

	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Close() error
	ExecWithRetry(ctx context.Context, query string, args ...any) (sql.Result, error)

	GetUser(userID int64) (*User, error)
	SaveUser(user User) error

	// Chat models management
	SaveChatModel(chatID int64, model string) error
	GetChatModel(chatID int64) (string, error)
	DeleteChatModel(chatID int64) error
	LoadAllChatModels() (map[int64]string, error)

	// Message storage
	SaveMessage(chatID int64, messageID int, username, mediaGroupID string, main bool, data []byte) error
	UpdateMessage(chatID int64, messageID int, data []byte) error
	GetMessage(chatID int64, messageID int) (*telegram.Update, error)
	GetMessagesBy(chatID int64, ignoreMessageID int64, duration time.Duration, count int, username string) ([]telegram.Update, error)
	GetMessagesWithMediaGroupID(chatID int64, mediaGroupID string) ([]telegram.Update, error)
	PurgeOldMessages(retentionDays int) error
	PurgeOldTasks(retentionDays int) error
}

type User struct {
	ID        int64     `json:"id"`
	PublicID  string    `json:"public_id"`
	FirstName string    `json:"first_name"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (u User) Equal(user User) bool {
	return u.FirstName == user.FirstName && u.Username == user.Username && user.PublicID != ""
}
