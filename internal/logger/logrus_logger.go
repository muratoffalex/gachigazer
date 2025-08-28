package logger

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/muratoffalex/gachigazer/internal/config"
)

type logrusLogger struct {
	logger logrus.Ext1FieldLogger
}

func NewLogrusLogger(cfg *config.LoggingConfig) Logger {
	l := logrus.New()
	l.SetFormatter(&logrus.TextFormatter{
		DisableQuote:    true,
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	level := logrus.WarnLevel
	l.SetLevel(level)
	level, err := logrus.ParseLevel(cfg.Level())
	if err != nil {
		l.WithFields(logrus.Fields{
			"log_level": cfg.Level(),
		}).Warn("Log level not found. Fallback to 'info'")
		level = logrus.InfoLevel
	}
	l.SetLevel(level)

	if cfg.WriteInFile {
		file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err == nil {
			mw := io.MultiWriter(os.Stdout, file)
			l.SetOutput(mw)
		} else {
			l.Info("Failed to log to file, using default stderr")
		}
	}

	return &logrusLogger{
		logger: l,
	}
}

func (l *logrusLogger) Trace(args ...any) {
	l.logger.Trace(args...)
}

func (l *logrusLogger) Debug(args ...any) {
	l.logger.Debug(args...)
}

func (l *logrusLogger) Info(args ...any) {
	l.logger.Info(args...)
}

func (l *logrusLogger) Warn(args ...any) {
	l.logger.Warn(args...)
}

func (l *logrusLogger) Error(args ...any) {
	l.logger.Error(args...)
}

func (l *logrusLogger) Fatal(args ...any) {
	l.logger.Fatal(args...)
}

func (l *logrusLogger) WithFields(fields Fields) Logger {
	return &logrusLogger{
		logger: l.logger.WithFields(logrus.Fields(fields)),
	}
}

func (l *logrusLogger) WithField(key string, value any) Logger {
	return &logrusLogger{
		logger: l.logger.WithField(key, value),
	}
}

func (l *logrusLogger) WithError(err error) Logger {
	return &logrusLogger{
		logger: l.logger.WithError(err),
	}
}
