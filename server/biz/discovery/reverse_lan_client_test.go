package discovery

import (
	"net"
	"testing"

	"github.com/grandcat/zeroconf"
)

const (
	reverseTestServerID   = "0123456789abcdef0123456789abcdef"
	reverseTestClientID   = "11111111-1111-4111-8111-111111111111"
	reverseTestGeneration = "22222222-2222-4222-8222-222222222222"
)

func TestParseReverseLanCandidate(t *testing.T) {
	entry := validReverseLanEntry()
	entry.AddrIPv4 = append(entry.AddrIPv4, net.ParseIP("8.8.8.8"))
	candidate, err := ParseReverseLanCandidate(entry, reverseTestServerID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.ClientID != reverseTestClientID ||
		candidate.Generation != reverseTestGeneration ||
		candidate.Port != 34567 {
		t.Fatalf("candidate=%+v", candidate)
	}
	if len(candidate.Addresses) != 1 || !candidate.Addresses[0].Equal(net.ParseIP("192.168.1.77")) {
		t.Fatalf("addresses=%v", candidate.Addresses)
	}
}

func TestParseReverseLanCandidateRejectsUntrustedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		change func(*zeroconf.ServiceEntry)
	}{
		{
			name: "server mismatch",
			change: func(entry *zeroconf.ServiceEntry) {
				entry.Text[3] = "server=abcdef0123456789abcdef0123456789"
			},
		},
		{
			name: "public address only",
			change: func(entry *zeroconf.ServiceEntry) {
				entry.AddrIPv4 = []net.IP{net.ParseIP("8.8.8.8")}
			},
		},
		{
			name: "duplicate metadata",
			change: func(entry *zeroconf.ServiceEntry) {
				entry.Text = append(entry.Text, "api=1")
			},
		},
		{
			name: "invalid port",
			change: func(entry *zeroconf.ServiceEntry) {
				entry.Port = 0
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry := validReverseLanEntry()
			test.change(entry)
			if _, err := ParseReverseLanCandidate(entry, reverseTestServerID); err == nil {
				t.Fatal("candidate accepted")
			}
		})
	}
}

func validReverseLanEntry() *zeroconf.ServiceEntry {
	return &zeroconf.ServiceEntry{
		Port: 34567,
		Text: []string{
			"api=1",
			"client=" + reverseTestClientID,
			"generation=" + reverseTestGeneration,
			"server=" + reverseTestServerID,
		},
		AddrIPv4: []net.IP{net.ParseIP("192.168.1.77")},
	}
}
