package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	ContextSyncServiceType = "_orange-context._tcp"
	ContextSyncDomain      = "local."
	interfacePollInterval  = 2 * time.Second
	mdnsShutdownTimeout    = 2 * time.Second
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
	interfaces, _, err := currentLANInterfaces()
	if err != nil {
		return nil, err
	}
	return startContextSyncServiceOnInterfaces(serverID, port, interfaces)
}

func startContextSyncServiceOnInterfaces(
	serverID string,
	port int,
	interfaces []net.Interface,
) (*zeroconf.Server, error) {
	instance, text, err := ContextSyncRegistration(serverID, port)
	if err != nil {
		return nil, err
	}
	if len(interfaces) == 0 {
		return nil, fmt.Errorf("no eligible LAN interface")
	}
	return zeroconf.Register(
		instance,
		ContextSyncServiceType,
		ContextSyncDomain,
		port,
		text,
		interfaces,
	)
}

type shutdowner interface {
	Shutdown()
}

type interfaceCandidate struct {
	iface net.Interface
	addrs []net.Addr
}

type contextSyncRegistrar func(string, int, []net.Interface) (shutdowner, error)
type lanInterfaceProvider func() ([]net.Interface, string, error)

type ContextSyncServiceManager struct {
	serverID          string
	port              int
	pollInterval      time.Duration
	shutdownTimeout   time.Duration
	interfaceProvider lanInterfaceProvider
	register          contextSyncRegistrar

	mu          sync.Mutex
	server      shutdowner
	fingerprint string
	cancel      context.CancelFunc
	done        chan struct{}
}

func StartContextSyncServiceManager(serverID string, port int) (*ContextSyncServiceManager, error) {
	if _, _, err := ContextSyncRegistration(serverID, port); err != nil {
		return nil, err
	}
	manager := &ContextSyncServiceManager{
		serverID:          serverID,
		port:              port,
		pollInterval:      interfacePollInterval,
		shutdownTimeout:   mdnsShutdownTimeout,
		interfaceProvider: currentLANInterfaces,
		register: func(serverID string, port int, interfaces []net.Interface) (shutdowner, error) {
			return startContextSyncServiceOnInterfaces(serverID, port, interfaces)
		},
		done: make(chan struct{}),
	}
	manager.refresh()
	ctx, cancel := context.WithCancel(context.Background())
	manager.cancel = cancel
	go manager.watch(ctx)
	return manager, nil
}

func (manager *ContextSyncServiceManager) watch(ctx context.Context) {
	// #region debug-point D:mdns-watch-panic
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[debug:zeronews-loopback-refused] mdns_watch_panic value=%v stack=%s",
				recovered, debug.Stack())
			panic(recovered)
		}
	}()
	// #endregion
	defer close(manager.done)
	ticker := time.NewTicker(manager.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			manager.refresh()
		}
	}
}

func (manager *ContextSyncServiceManager) refresh() {
	interfaces, fingerprint, err := manager.interfaceProvider()
	if err != nil {
		log.Printf("[context-video-sync] mdns_unavailable reason=interface_scan_failed err=%v", err)
		return
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.server != nil && manager.fingerprint == fingerprint {
		return
	}
	if manager.fingerprint != fingerprint {
		log.Printf("[context-video-sync] interface_snapshot_changed old=%q new=%q",
			manager.fingerprint, fingerprint)
	}
	if manager.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), manager.shutdownTimeout)
		err = ShutdownContextSyncService(ctx, manager.server)
		cancel()
		if err != nil {
			log.Printf("[context-video-sync] mDNS shutdown timed out during re-registration: %v", err)
		}
		manager.server = nil
	}
	manager.fingerprint = fingerprint
	if len(interfaces) == 0 {
		log.Printf("[context-video-sync] mdns_unavailable reason=no_lan_interface")
		return
	}
	server, err := manager.register(manager.serverID, manager.port, interfaces)
	if err != nil {
		log.Printf("[context-video-sync] mdns_unavailable reason=registration_failed interfaces=%q err=%v",
			fingerprint, err)
		return
	}
	manager.server = server
	log.Printf("[context-video-sync] mdns_registered service=%s port=%d interfaces=%q",
		ContextSyncServiceType, manager.port, fingerprint)
}

func (manager *ContextSyncServiceManager) Shutdown(ctx context.Context) error {
	if manager == nil {
		return nil
	}
	if manager.cancel != nil {
		manager.cancel()
	}
	select {
	case <-manager.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return ShutdownContextSyncService(ctx, manager.server)
}

func currentLANInterfaces() ([]net.Interface, string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, "", err
	}
	candidates := make([]interfaceCandidate, 0, len(interfaces))
	for _, iface := range interfaces {
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		candidates = append(candidates, interfaceCandidate{iface: iface, addrs: addrs})
	}
	return selectLANInterfaces(candidates)
}

func selectLANInterfaces(candidates []interfaceCandidate) ([]net.Interface, string, error) {
	interfaces := make([]net.Interface, 0, len(candidates))
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		flags := candidate.iface.Flags
		if flags&net.FlagUp == 0 || flags&net.FlagMulticast == 0 ||
			flags&net.FlagLoopback != 0 || flags&net.FlagPointToPoint != 0 {
			continue
		}
		addresses := make([]string, 0, len(candidate.addrs))
		for _, address := range candidate.addrs {
			ip, _, err := net.ParseCIDR(address.String())
				if err != nil || ip == nil || !ip.IsGlobalUnicast() || ip.IsLinkLocalUnicast() {
				continue
			}
			addresses = append(addresses, ip.String())
		}
		if len(addresses) == 0 {
			continue
		}
		sort.Strings(addresses)
		interfaces = append(interfaces, candidate.iface)
		parts = append(parts, fmt.Sprintf("%s:%s", candidate.iface.Name, strings.Join(addresses, ",")))
	}
	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].Name < interfaces[j].Name })
	sort.Strings(parts)
	return interfaces, strings.Join(parts, ";"), nil
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
