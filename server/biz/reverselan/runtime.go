package reverselan

import (
	"context"
	"fmt"

	"aaa_word/biz/discovery"
)

type Runtime struct {
	cancel  context.CancelFunc
	bridges *BridgeManager
}

func StartRuntime(serverID string, serverPort int, registry *Registry) (*Runtime, error) {
	if err := ValidateServerID(serverID); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	bridges := NewBridgeManager(
		registry,
		NewForwarder(serverPort, NewPolicyRegistry(LearningPolicies()...)),
	)
	if err := discovery.StartReverseLanBrowser(ctx, serverID, bridges.HandleCandidate); err != nil {
		cancel()
		bridges.Shutdown()
		return nil, fmt.Errorf("start reverse LAN discovery: %w", err)
	}
	return &Runtime{cancel: cancel, bridges: bridges}, nil
}

func (runtime *Runtime) Shutdown() {
	if runtime == nil {
		return
	}
	runtime.cancel()
	runtime.bridges.Shutdown()
}
