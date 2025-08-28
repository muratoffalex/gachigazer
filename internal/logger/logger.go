package logger

type Fields map[string]any

type Logger interface {
	Trace(args ...any)
	Debug(args ...any)
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)
	Fatal(args ...any)

	WithFields(fields Fields) Logger
	WithField(key string, value any) Logger
	WithError(err error) Logger
}
