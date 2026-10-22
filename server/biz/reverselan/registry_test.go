package reverselan

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

const (
	testServerID   = "0123456789abcdef0123456789abcdef"
	testClientID   = "11111111-1111-4111-8111-111111111111"
	testGeneration = "22222222-2222-4222-8222-222222222222"
)

func TestRegistryRegistrationLifecycle(t *testing.T) {
	now := time.Unix(100, 0)
	registry := NewRegistry(testServerID, time.Minute, 2)
	registry.now = func() time.Time { return now }
	registration := validRegistration()

	first, err := registry.Register(registration)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Second)
	second, err := registry.Register(registration)
	if err != nil {
		t.Fatal(err)
	}
	if !second.ExpiresAt.After(first.ExpiresAt) {
		t.Fatal("same client/generation did not refresh ticket")
	}
	now = second.ExpiresAt
	if _, ok := registry.Lookup(testClientID, testGeneration); ok {
		t.Fatal("expired ticket remained available")
	}
}

func TestRegistryRejectsInvalidOrMismatchedRegistration(t *testing.T) {
	registry := NewRegistry(testServerID, time.Minute, 2)
	tests := []struct {
		name          string
		change        func(*Registration)
		expectedError error
	}{
		{
			name: "server mismatch",
			change: func(value *Registration) {
				value.ServerID = "abcdef0123456789abcdef0123456789"
			},
			expectedError: ErrRegistrationMismatch,
		},
		{
			name: "invalid client",
			change: func(value *Registration) {
				value.ClientID = "not-a-uuid"
			},
			expectedError: ErrRegistrationInvalid,
		},
		{
			name: "short secret",
			change: func(value *Registration) {
				value.Secret = base64.RawURLEncoding.EncodeToString([]byte("short"))
			},
			expectedError: ErrRegistrationInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validRegistration()
			test.change(&value)
			_, err := registry.Register(value)
			if !errors.Is(err, test.expectedError) {
				t.Fatalf("error=%v, want %v", err, test.expectedError)
			}
		})
	}
}

func TestRegistryCapacityEvictsOldestTicket(t *testing.T) {
	now := time.Unix(100, 0)
	registry := NewRegistry(testServerID, time.Minute, 2)
	registry.now = func() time.Time { return now }

	one := validRegistration()
	two := validRegistration()
	two.ClientID = "33333333-3333-4333-8333-333333333333"
	three := validRegistration()
	three.ClientID = "44444444-4444-4444-8444-444444444444"
	if _, err := registry.Register(one); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if _, err := registry.Register(two); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if _, err := registry.Register(three); err != nil {
		t.Fatal(err)
	}

	if _, ok := registry.Lookup(one.ClientID, one.Generation); ok {
		t.Fatal("oldest ticket was not evicted")
	}
	if _, ok := registry.Lookup(two.ClientID, two.Generation); !ok {
		t.Fatal("newer ticket was evicted")
	}
	if _, ok := registry.Lookup(three.ClientID, three.Generation); !ok {
		t.Fatal("newest ticket missing")
	}
}

func validRegistration() Registration {
	return Registration{
		APIVersion: APIVersion,
		ClientID:   testClientID,
		ServerID:   testServerID,
		Generation: testGeneration,
		Secret:     base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
	}
}
