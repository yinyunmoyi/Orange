package reverselan

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"
)

const (
	DefaultTicketTTL      = 60 * time.Second
	DefaultTicketCapacity = 64
)

var (
	ErrRegistrationInvalid  = errors.New("invalid reverse LAN registration")
	ErrRegistrationMismatch = errors.New("reverse LAN server identity mismatch")
	uuidPattern             = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	serverIDPattern         = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type Registration struct {
	APIVersion int
	ClientID   string
	ServerID   string
	Generation string
	Secret     string
}

type Ticket struct {
	ClientID   string
	ServerID   string
	Generation string
	Secret     string
	ExpiresAt  time.Time
}

type Registry struct {
	mu       sync.Mutex
	serverID string
	ttl      time.Duration
	capacity int
	now      func() time.Time
	tickets  map[string]Ticket
}

func NewRegistry(serverID string, ttl time.Duration, capacity int) *Registry {
	return &Registry{
		serverID: serverID,
		ttl:      ttl,
		capacity: capacity,
		now:      time.Now,
		tickets:  make(map[string]Ticket),
	}
}

func (registry *Registry) Register(registration Registration) (Ticket, error) {
	if registration.APIVersion != APIVersion ||
		!uuidPattern.MatchString(registration.ClientID) ||
		!uuidPattern.MatchString(registration.Generation) ||
		!serverIDPattern.MatchString(registration.ServerID) ||
		!validSecret(registration.Secret) {
		return Ticket{}, ErrRegistrationInvalid
	}
	if registration.ServerID != registry.serverID {
		return Ticket{}, ErrRegistrationMismatch
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	now := registry.now()
	registry.removeExpiredLocked(now)
	key := ticketKey(registration.ClientID, registration.Generation)
	if _, exists := registry.tickets[key]; !exists && len(registry.tickets) >= registry.capacity {
		registry.removeOldestLocked()
	}
	ticket := Ticket{
		ClientID:   registration.ClientID,
		ServerID:   registration.ServerID,
		Generation: registration.Generation,
		Secret:     registration.Secret,
		ExpiresAt:  now.Add(registry.ttl),
	}
	registry.tickets[key] = ticket
	return ticket, nil
}

func (registry *Registry) Lookup(clientID, generation string) (Ticket, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	now := registry.now()
	registry.removeExpiredLocked(now)
	ticket, ok := registry.tickets[ticketKey(clientID, generation)]
	return ticket, ok
}

func (registry *Registry) removeExpiredLocked(now time.Time) {
	for key, ticket := range registry.tickets {
		if !ticket.ExpiresAt.After(now) {
			delete(registry.tickets, key)
		}
	}
}

func (registry *Registry) removeOldestLocked() {
	var oldestKey string
	var oldestExpiry time.Time
	for key, ticket := range registry.tickets {
		if oldestKey == "" || ticket.ExpiresAt.Before(oldestExpiry) {
			oldestKey = key
			oldestExpiry = ticket.ExpiresAt
		}
	}
	if oldestKey != "" {
		delete(registry.tickets, oldestKey)
	}
}

func ticketKey(clientID, generation string) string {
	return clientID + "/" + generation
}

func validSecret(secret string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(secret)
	return err == nil && len(decoded) == 32
}

func ValidateServerID(serverID string) error {
	if !serverIDPattern.MatchString(serverID) {
		return fmt.Errorf("%w: server id", ErrRegistrationInvalid)
	}
	return nil
}
