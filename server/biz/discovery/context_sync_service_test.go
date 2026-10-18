package discovery

import (
	"context"
	"errors"
	"net"
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

func TestSelectLANInterfacesFiltersAndStabilizesSnapshot(t *testing.T) {
	candidates := []interfaceCandidate{
		{
			iface: net.Interface{Name: "utun3", Flags: net.FlagUp | net.FlagMulticast | net.FlagPointToPoint},
			addrs: []net.Addr{cidr(t, "10.4.8.220/20")},
		},
		{
			iface: net.Interface{Name: "en0", Flags: net.FlagUp | net.FlagMulticast},
			addrs: []net.Addr{cidr(t, "fe80::1/64"), cidr(t, "192.168.1.6/24")},
		},
		{
			iface: net.Interface{Name: "awdl0", Flags: net.FlagUp | net.FlagMulticast},
			addrs: []net.Addr{cidr(t, "fe80::d015:88ff:fe1c:e079/64")},
		},
		{
			iface: net.Interface{Name: "lo0", Flags: net.FlagUp | net.FlagMulticast | net.FlagLoopback},
			addrs: []net.Addr{cidr(t, "127.0.0.1/8")},
		},
	}

	interfaces, fingerprint, err := selectLANInterfaces(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(interfaces) != 1 || interfaces[0].Name != "en0" {
		t.Fatalf("interfaces=%v, want en0 only", interfaces)
	}
	if fingerprint != "en0:192.168.1.6" {
		t.Fatalf("fingerprint=%q", fingerprint)
	}
}

func TestContextSyncManagerReregistersOnlyWhenSnapshotChanges(t *testing.T) {
	snapshots := []struct {
		interfaces  []net.Interface
		fingerprint string
	}{
		{[]net.Interface{{Name: "en0"}}, "en0:192.168.1.6"},
		{[]net.Interface{{Name: "en0"}}, "en0:192.168.1.6"},
		{[]net.Interface{{Name: "en0"}}, "en0:192.168.1.7"},
	}
	index := 0
	registered := 0
	first := &countingShutdown{}
	second := &countingShutdown{}
	manager := &ContextSyncServiceManager{
		serverID:        "0123456789abcdef0123456789abcdef",
		port:            8888,
		shutdownTimeout: time.Second,
		interfaceProvider: func() ([]net.Interface, string, error) {
			value := snapshots[index]
			if index < len(snapshots)-1 {
				index++
			}
			return value.interfaces, value.fingerprint, nil
		},
		register: func(_ string, _ int, _ []net.Interface) (shutdowner, error) {
			registered++
			if registered == 1 {
				return first, nil
			}
			return second, nil
		},
	}

	manager.refresh()
	manager.refresh()
	manager.refresh()

	if registered != 2 {
		t.Fatalf("registered=%d, want 2", registered)
	}
	if first.calls != 1 {
		t.Fatalf("first shutdown calls=%d, want 1", first.calls)
	}
	if second.calls != 0 {
		t.Fatalf("second shutdown calls=%d, want 0", second.calls)
	}
}

func TestContextSyncManagerRetriesRegistrationWithoutInterfaceChange(t *testing.T) {
	attempts := 0
	manager := &ContextSyncServiceManager{
		serverID:        "0123456789abcdef0123456789abcdef",
		port:            8888,
		shutdownTimeout: time.Second,
		interfaceProvider: func() ([]net.Interface, string, error) {
			return []net.Interface{{Name: "en0"}}, "en0:192.168.1.6", nil
		},
		register: func(_ string, _ int, _ []net.Interface) (shutdowner, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("temporary registration failure")
			}
			return &countingShutdown{}, nil
		},
	}

	manager.refresh()
	manager.refresh()

	if attempts != 2 {
		t.Fatalf("registration attempts=%d, want 2", attempts)
	}
	if manager.server == nil {
		t.Fatal("manager did not recover from registration failure")
	}
}

func cidr(t *testing.T, value string) net.Addr {
	t.Helper()
	ip, network, err := net.ParseCIDR(value)
	if err != nil {
		t.Fatal(err)
	}
	network.IP = ip
	return network
}

type countingShutdown struct {
	calls int
}

func (stub *countingShutdown) Shutdown() {
	stub.calls++
}
