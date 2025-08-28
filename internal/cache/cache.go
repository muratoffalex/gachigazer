package cache

import "time"

type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, data []byte, ttl time.Duration) error
	Delete(key string) error
	Clear() error
}
