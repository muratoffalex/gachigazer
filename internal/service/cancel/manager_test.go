package cancel

import (
	"context"
	"testing"
	"time"
)

func TestManager_RegisterAndUnregister(t *testing.T) {
	m := NewManager()

	ctx, unregister := m.Register(123, 456, "ask")
	if ctx == nil {
		t.Error("expected non-nil context")
	}

	if !m.IsActive(123, 456) {
		t.Error("expected request to be active")
	}

	info := m.GetActiveRequest(123, 456)
	if info == nil {
		t.Fatal("expected non-nil request info")
	}
	if info.ChatID != 123 {
		t.Errorf("expected ChatID 123, got %d", info.ChatID)
	}
	if info.MessageID != 456 {
		t.Errorf("expected MessageID 456, got %d", info.MessageID)
	}
	if info.Command != "ask" {
		t.Errorf("expected Command 'ask', got %s", info.Command)
	}

	unregister()

	if m.IsActive(123, 456) {
		t.Error("expected request to be inactive after unregister")
	}
}

func TestManager_Cancel(t *testing.T) {
	m := NewManager()

	ctx, unregister := m.Register(123, 456, "ask")
	defer unregister()

	done := make(chan bool)
	go func() {
		<-ctx.Done()
		done <- true
	}()

	if !m.Cancel(123, 456) {
		t.Error("expected Cancel to return true")
	}

	select {
	case <-done:
		// Context was cancelled, good
	case <-time.After(time.Second):
		t.Error("expected context to be cancelled")
	}

	if ctx.Err() != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", ctx.Err())
	}
}

func TestManager_Cancel_NotFound(t *testing.T) {
	m := NewManager()

	if m.Cancel(123, 999) {
		t.Error("expected Cancel to return false for non-existent request")
	}
}

func TestManager_UpdateProgress(t *testing.T) {
	m := NewManager()

	_, unregister := m.Register(123, 456, "ask")
	defer unregister()

	m.UpdateProgress(123, 456, "partial response")

	progress := m.GetProgress(123, 456)
	if progress != "partial response" {
		t.Errorf("expected progress 'partial response', got '%s'", progress)
	}

	info := m.GetActiveRequest(123, 456)
	if info.Progress != "partial response" {
		t.Errorf("expected info.Progress 'partial response', got '%s'", info.Progress)
	}
}

func TestManager_UpdateProgress_NotFound(t *testing.T) {
	m := NewManager()

	// Should not panic
	m.UpdateProgress(123, 999, "progress")

	progress := m.GetProgress(123, 999)
	if progress != "" {
		t.Errorf("expected empty progress for non-existent request, got '%s'", progress)
	}
}

func TestManager_MultipleRequests(t *testing.T) {
	m := NewManager()

	ctx1, cancel1 := m.Register(123, 1, "ask")
	ctx2, cancel2 := m.Register(123, 2, "ask")
	ctx3, cancel3 := m.Register(456, 1, "image")

	if !m.IsActive(123, 1) || !m.IsActive(123, 2) || !m.IsActive(456, 1) {
		t.Error("expected all requests to be active")
	}

	// Cancel only request 2
	if !m.Cancel(123, 2) {
		t.Error("expected Cancel to return true")
	}

	if ctx1.Err() != nil {
		t.Error("expected ctx1 to not be cancelled")
	}
	if ctx2.Err() == nil {
		t.Error("expected ctx2 to be cancelled")
	}
	if ctx3.Err() != nil {
		t.Error("expected ctx3 to not be cancelled")
	}

	// Cancel doesn't unregister, so request is still "active" in map
	// but context is cancelled. Unregister removes from map.
	cancel2()

	if m.IsActive(123, 2) {
		t.Error("expected request 2 to be inactive after unregister")
	}
	if !m.IsActive(123, 1) || !m.IsActive(456, 1) {
		t.Error("expected requests 1 and 3 to still be active")
	}

	cancel1()
	cancel2()
	cancel3()
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager()

	// Register multiple requests concurrently
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(id int) {
			_, unregister := m.Register(int64(id), id, "ask")
			time.Sleep(time.Millisecond)
			unregister()
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	// All requests should be cleaned up
	for i := 0; i < 100; i++ {
		if m.IsActive(int64(i), i) {
			t.Errorf("expected request %d to be inactive", i)
		}
	}
}
