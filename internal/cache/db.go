package cache

import (
	"time"

	"github.com/muratoffalex/gachigazer/internal/database"
)

type DBCache struct {
	db database.Database
}

func NewDBCache(db database.Database) Cache {
	return &DBCache{db: db}
}

func (c *DBCache) Get(key string) ([]byte, bool) {
	var data []byte
	var expiresAt time.Time

	err := c.db.QueryRow(`
        SELECT data, expires_at 
        FROM cache 
        WHERE key = ?
    `, key).Scan(&data, &expiresAt)

	if err != nil {
		return nil, false
	}

	if time.Now().After(expiresAt) {
		c.Delete(key)
		return nil, false
	}

	return data, true
}

func (c *DBCache) Set(key string, data []byte, ttl time.Duration) error {
	_, err := c.db.Exec(`
        INSERT OR REPLACE INTO cache (key, data, expires_at)
        VALUES (?, ?, ?)
    `, key, data, time.Now().Add(ttl))
	return err
}

func (c *DBCache) Delete(key string) error {
	_, err := c.db.Exec("DELETE FROM cache WHERE key = ?", key)
	return err
}

func (c *DBCache) Clear() error {
	_, err := c.db.Exec("DELETE FROM cache")
	return err
}
