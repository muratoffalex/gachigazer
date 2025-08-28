package cache

import (
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/logger"
)

type MultiLevelCache struct {
	memory Cache
	db     Cache
	logger logger.Logger
}

func NewMultiLevelCache(memory, db Cache, logger logger.Logger) Cache {
	return &MultiLevelCache{
		memory: memory,
		db:     db,
		logger: logger,
	}
}

const (
	MemoryOnlyPrefix = "mem:"
	PersistentPrefix = "db:"
)

func (c *MultiLevelCache) Get(key string) ([]byte, bool) {
	if after, ok := strings.CutPrefix(key, MemoryOnlyPrefix); ok {
		// For keys with the mem: prefix, use memory only
		return c.memory.Get(after)
	}

	key = strings.TrimPrefix(key, PersistentPrefix)

	// Standard multi-level cache logic
	if data, found := c.memory.Get(key); found {
		return data, true
	}

	if data, found := c.db.Get(key); found {
		_ = c.memory.Set(key, data, 24*time.Hour)
		return data, true
	}

	return nil, false
}

func (c *MultiLevelCache) Set(key string, data []byte, ttl time.Duration) error {
	if after, ok := strings.CutPrefix(key, MemoryOnlyPrefix); ok {
		// For keys with the mem: prefix, use memory only
		return c.memory.Set(after, data, ttl)
	}

	key = strings.TrimPrefix(key, PersistentPrefix)

	// Standard multi-level cache logic
	if err := c.db.Set(key, data, ttl); err != nil {
		return err
	}
	_ = c.memory.Set(key, data, ttl)
	return nil
}

func (c *MultiLevelCache) Delete(key string) error {
	// Delete from both levels
	if err := c.memory.Delete(key); err != nil {
		c.logger.WithError(err).Error("Failed to delete from memory cache")
	}

	if err := c.db.Delete(key); err != nil {
		c.logger.WithError(err).Error("Failed to delete from db cache")
		return err
	}

	return nil
}

func (c *MultiLevelCache) Clear() error {
	// Clear both levels
	if err := c.memory.Clear(); err != nil {
		c.logger.WithError(err).Error("Failed to clear memory cache")
	}

	if err := c.db.Clear(); err != nil {
		c.logger.WithError(err).Error("Failed to clear db cache")
		return err
	}

	return nil
}
