package commands

import (
	"time"

	"github.com/muratoffalex/gachigazer/internal/telegram"
)

type Command interface {
	Name() string
	Aliases() []string
	Handle(update telegram.Update) error
	Execute(update telegram.Update) error
	GetQueueConfig() QueueConfig
}

type ThrottleConfig struct {
	Period      time.Duration
	Requests    int
	Concurrency int
}

type QueueConfig struct {
	MaxRetries int
	RetryDelay time.Duration
	Timeout    time.Duration
	Throttle   ThrottleConfig
}
