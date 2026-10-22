package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/grandcat/zeroconf"
)

const ReverseLanServiceType = "_orange-client._tcp"

var ErrInvalidReverseLanCandidate = errors.New("invalid reverse LAN mDNS candidate")

type ReverseLanCandidate struct {
	ClientID   string
	ServerID   string
	Generation string
	Port       int
	Addresses  []net.IP
}

func StartReverseLanBrowser(
	ctx context.Context,
	serverID string,
	onCandidate func(ReverseLanCandidate),
) error {
	resolver, err := zeroconf.NewResolver()
	if err != nil {
		return fmt.Errorf("create reverse LAN resolver: %w", err)
	}
	entries := make(chan *zeroconf.ServiceEntry, 16)
	if err := resolver.Browse(ctx, ReverseLanServiceType, ContextSyncDomain, entries); err != nil {
		return fmt.Errorf("browse reverse LAN service: %w", err)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case entry, ok := <-entries:
				if !ok {
					return
				}
				candidate, parseErr := ParseReverseLanCandidate(entry, serverID)
				if parseErr == nil {
					onCandidate(candidate)
				}
			}
		}
	}()
	return nil
}

func ParseReverseLanCandidate(entry *zeroconf.ServiceEntry, expectedServerID string) (ReverseLanCandidate, error) {
	if entry == nil || entry.Port < 1 || entry.Port > 65535 {
		return ReverseLanCandidate{}, ErrInvalidReverseLanCandidate
	}
	text, err := parseReverseLanText(entry.Text)
	if err != nil || text["api"] != strconv.Itoa(1) ||
		text["server"] != expectedServerID ||
		text["client"] == "" || text["generation"] == "" {
		return ReverseLanCandidate{}, ErrInvalidReverseLanCandidate
	}
	addresses := make([]net.IP, 0, len(entry.AddrIPv4)+len(entry.AddrIPv6))
	for _, address := range append(append([]net.IP(nil), entry.AddrIPv4...), entry.AddrIPv6...) {
		if isReverseLanAddress(address) {
			addresses = append(addresses, append(net.IP(nil), address...))
		}
	}
	if len(addresses) == 0 {
		return ReverseLanCandidate{}, ErrInvalidReverseLanCandidate
	}
	return ReverseLanCandidate{
		ClientID:   text["client"],
		ServerID:   text["server"],
		Generation: text["generation"],
		Port:       entry.Port,
		Addresses:  addresses,
	}, nil
}

func parseReverseLanText(values []string) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if !ok || key == "" || item == "" {
			return nil, ErrInvalidReverseLanCandidate
		}
		if _, exists := result[key]; exists {
			return nil, ErrInvalidReverseLanCandidate
		}
		result[key] = item
	}
	return result, nil
}

func isReverseLanAddress(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
