package handler

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"aaa_word/biz/reverselan"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestRegisterReverseLanClient(t *testing.T) {
	const (
		serverID   = "0123456789abcdef0123456789abcdef"
		clientID   = "11111111-1111-4111-8111-111111111111"
		generation = "22222222-2222-4222-8222-222222222222"
	)
	secret := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	registry := reverselan.NewRegistry(serverID, reverselan.DefaultTicketTTL, 4)
	ConfigureReverseLanRegistration(true, registry)
	t.Cleanup(func() { ConfigureReverseLanRegistration(false, nil) })

	c := &app.RequestContext{}
	c.Request.SetBodyString(`{"apiVersion":1,"clientId":"` + clientID +
		`","serverId":"` + serverID + `","generation":"` + generation +
		`","secret":"` + secret + `"}`)
	RegisterReverseLanClient(context.Background(), c)

	if c.Response.StatusCode() != http.StatusOK {
		t.Fatalf("status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}
	if strings.Contains(string(c.Response.Body()), secret) {
		t.Fatal("response leaked registration secret")
	}
	if _, ok := registry.Lookup(clientID, generation); !ok {
		t.Fatal("ticket was not registered")
	}
}

func TestRegisterReverseLanClientRejectsUnknownFieldsAndDisabledState(t *testing.T) {
	registry := reverselan.NewRegistry(
		"0123456789abcdef0123456789abcdef",
		time.Minute,
		4,
	)
	ConfigureReverseLanRegistration(true, registry)
	t.Cleanup(func() { ConfigureReverseLanRegistration(false, nil) })
	c := &app.RequestContext{}
	c.Request.SetBodyString(`{"apiVersion":1,"ip":"192.168.1.77"}`)
	RegisterReverseLanClient(context.Background(), c)
	if c.Response.StatusCode() != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d", c.Response.StatusCode())
	}

	ConfigureReverseLanRegistration(false, nil)
	disabled := &app.RequestContext{}
	disabled.Request.SetBodyString(`{}`)
	RegisterReverseLanClient(context.Background(), disabled)
	if disabled.Response.StatusCode() != http.StatusNotFound {
		t.Fatalf("disabled status=%d", disabled.Response.StatusCode())
	}
}
