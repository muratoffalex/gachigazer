package logger

import (
	"fmt"
	"maps"
	"sync"
)

type testLoggerStorage struct {
	mu      sync.RWMutex
	entries []TestLogEntry
}

type TestLogger struct {
	storage *testLoggerStorage
	fields  Fields
}

type TestLogEntry struct {
	Level   string
	Message string
	Fields  Fields
}

func NewTestLogger() *TestLogger {
	return &TestLogger{
		storage: &testLoggerStorage{
			entries: make([]TestLogEntry, 0),
		},
		fields: make(Fields),
	}
}

func (l *TestLogger) addEntry(level, message string, fields Fields) {
	l.storage.mu.Lock()
	defer l.storage.mu.Unlock()

	mergedFields := make(Fields)
	maps.Copy(mergedFields, l.fields)
	maps.Copy(mergedFields, fields)

	l.storage.entries = append(l.storage.entries, TestLogEntry{
		Level:   level,
		Message: message,
		Fields:  mergedFields,
	})
}

func (l *TestLogger) Trace(args ...any) {
	l.addEntry("trace", fmt.Sprint(args...), nil)
}

func (l *TestLogger) Debug(args ...any) {
	l.addEntry("debug", fmt.Sprint(args...), nil)
}

func (l *TestLogger) Info(args ...any) {
	l.addEntry("info", fmt.Sprint(args...), nil)
}

func (l *TestLogger) Warn(args ...any) {
	l.addEntry("warn", fmt.Sprint(args...), nil)
}

func (l *TestLogger) Error(args ...any) {
	l.addEntry("error", fmt.Sprint(args...), nil)
}

func (l *TestLogger) Fatal(args ...any) {
	l.addEntry("fatal", fmt.Sprint(args...), nil)
}

func (l *TestLogger) WithFields(fields Fields) Logger {
	mergedFields := make(Fields)
	maps.Copy(mergedFields, l.fields)
	maps.Copy(mergedFields, fields)

	return &TestLogger{
		storage: l.storage,
		fields:  mergedFields,
	}
}

func (l *TestLogger) WithField(key string, value any) Logger {
	fields := Fields{key: value}
	return l.WithFields(fields)
}

func (l *TestLogger) WithError(err error) Logger {
	fields := Fields{"error": err}
	return l.WithFields(fields)
}

// Methods for testing

func (l *TestLogger) GetEntries() []TestLogEntry {
	l.storage.mu.RLock()
	defer l.storage.mu.RUnlock()
	return append([]TestLogEntry{}, l.storage.entries...)
}

func (l *TestLogger) Clear() {
	l.storage.mu.Lock()
	defer l.storage.mu.Unlock()
	l.storage.entries = make([]TestLogEntry, 0)
}

func (l *TestLogger) HasEntry(level, message string) bool {
	entries := l.GetEntries()
	for _, entry := range entries {
		if entry.Level == level && entry.Message == message {
			return true
		}
	}
	return false
}

func (l *TestLogger) CountEntries() int {
	l.storage.mu.RLock()
	defer l.storage.mu.RUnlock()
	return len(l.storage.entries)
}
