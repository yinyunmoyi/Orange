package discovery

import (
	"context"
	"fmt"

	"github.com/grandcat/zeroconf"
)

const (
	ContextSyncServiceType = "_orange-context._tcp"
	ContextSyncDomain      = "local."
)

func ContextSyncRegistration(serverID string, port int) (string, []string, error) {
	if len(serverID) < 8 || port <= 0 || port > 65535 {
		return "", nil, fmt.Errorf("invalid context sync registration")
	}
	return "Orange Context " + serverID[:8], []string{
		"id=" + serverID,
		"api=1",
	}, nil
}

func StartContextSyncService(serverID string, port int) (*zeroconf.Server, error) {
	instance, text, err := ContextSyncRegistration(serverID, port)
	if err != nil {
		return nil, err
	}
	return zeroconf.Register(
		instance,
		ContextSyncServiceType,
		ContextSyncDomain,
		port,
		text,
		nil,
	)
}

type shutdowner interface {
	Shutdown()
}

// ShutdownContextSyncService waits for zeroconf's goodbye broadcast, but lets
// process shutdown continue if the third-party implementation blocks.
func ShutdownContextSyncService(ctx context.Context, server shutdowner) error {
	if server == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Shutdown()
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
