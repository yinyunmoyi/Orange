package discovery

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestContextSyncRegistration(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	instance, text, err := ContextSyncRegistration(id, 8888)
	if err != nil {
		t.Fatal(err)
	}
	if instance != "Orange Context 01234567" {
		t.Fatalf("instance=%q", instance)
	}
	if len(text) != 2 || text[0] != "id="+id || text[1] != "api=1" {
		t.Fatalf("text=%v", text)
	}
}

func TestContextSyncRegistrationRejectsInvalidInput(t *testing.T) {
	if _, _, err := ContextSyncRegistration("short", 8888); err == nil {
		t.Fatal("short id accepted")
	}
	if _, _, err := ContextSyncRegistration("0123456789abcdef", 0); err == nil {
		t.Fatal("invalid port accepted")
	}
}

type shutdownStub struct {
	release chan struct{}
	called  chan struct{}
}

func (stub *shutdownStub) Shutdown() {
	close(stub.called)
	<-stub.release
}

func TestShutdownContextSyncServiceReturnsAfterShutdown(t *testing.T) {
	stub := &shutdownStub{
		release: make(chan struct{}),
		called:  make(chan struct{}),
	}
	close(stub.release)

	if err := ShutdownContextSyncService(context.Background(), stub); err != nil {
		t.Fatalf("ShutdownContextSyncService() error = %v", err)
	}
	select {
	case <-stub.called:
	default:
		t.Fatal("Shutdown was not called")
	}
}

func TestShutdownContextSyncServiceHonorsTimeout(t *testing.T) {
	stub := &shutdownStub{
		release: make(chan struct{}),
		called:  make(chan struct{}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := ShutdownContextSyncService(ctx, stub)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	select {
	case <-stub.called:
	default:
		t.Fatal("Shutdown was not called")
	}
	close(stub.release)
}
