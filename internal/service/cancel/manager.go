package cancel

import (
	"context"
	"fmt"
	"sync"
)

type Manager struct {
	requests map[string]*activeRequest
	mu       sync.RWMutex
}

type activeRequest struct {
	cancel    context.CancelFunc
	chatID    int64
	messageID int
	command   string
	progress  string
}

func NewManager() *Manager {
	return &Manager{
		requests: make(map[string]*activeRequest),
	}
}

func (m *Manager) makeKey(chatID int64, messageID int) string {
	return fmt.Sprintf("%d:%d", chatID, messageID)
}

func (m *Manager) Register(chatID int64, messageID int, command string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.makeKey(chatID, messageID)
	m.requests[key] = &activeRequest{
		cancel:    cancel,
		chatID:    chatID,
		messageID: messageID,
		command:   command,
	}

	unregister := func() {
		cancel()
		m.Unregister(chatID, messageID)
	}

	return ctx, unregister
}

func (m *Manager) Unregister(chatID int64, messageID int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.makeKey(chatID, messageID)
	delete(m.requests, key)
}

func (m *Manager) Cancel(chatID int64, messageID int) bool {
	m.mu.RLock()
	req, exists := m.requests[m.makeKey(chatID, messageID)]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	req.cancel()
	return true
}

func (m *Manager) IsActive(chatID int64, messageID int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.requests[m.makeKey(chatID, messageID)]
	return exists
}

func (m *Manager) GetActiveRequest(chatID int64, messageID int) *ActiveRequestInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	req, exists := m.requests[m.makeKey(chatID, messageID)]
	if !exists {
		return nil
	}

	return &ActiveRequestInfo{
		ChatID:    req.chatID,
		MessageID: req.messageID,
		Command:   req.command,
		Progress:  req.progress,
	}
}

type ActiveRequestInfo struct {
	ChatID    int64
	MessageID int
	Command   string
	Progress  string
}

func (m *Manager) UpdateProgress(chatID int64, messageID int, progress string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.makeKey(chatID, messageID)
	if req, exists := m.requests[key]; exists {
		req.progress = progress
	}
}

func (m *Manager) GetProgress(chatID int64, messageID int) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.makeKey(chatID, messageID)
	if req, exists := m.requests[key]; exists {
		return req.progress
	}
	return ""
}
