package main

import (
	"os"
	"syscall"
	"testing"
)

func TestServerShutdownSignalsExcludeSIGHUP(t *testing.T) {
	signals := serverShutdownSignals()
	if len(signals) != 2 {
		t.Fatalf("signals=%v, want SIGINT and SIGTERM", signals)
	}
	expected := map[os.Signal]bool{
		syscall.SIGINT:  true,
		syscall.SIGTERM: true,
	}
	for _, current := range signals {
		if !expected[current] {
			t.Fatalf("unexpected shutdown signal %v", current)
		}
		delete(expected, current)
	}
	if len(expected) != 0 {
		t.Fatalf("missing shutdown signals: %v", expected)
	}
}
