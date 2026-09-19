package handler

import (
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestSyncPositiveQueryInt(t *testing.T) {
	if got, err := syncPositiveQueryInt("", 7, 7); err != nil || got != 7 {
		t.Fatalf("default=(%d,%v)", got, err)
	}
	if got, err := syncPositiveQueryInt("3", 7, 7); err != nil || got != 3 {
		t.Fatalf("value=(%d,%v)", got, err)
	}
	for _, raw := range []string{"0", "8", "bad"} {
		if _, err := syncPositiveQueryInt(raw, 7, 7); err == nil {
			t.Fatalf("raw=%q accepted", raw)
		}
	}
}

func TestContextVideoNotModified(t *testing.T) {
	c := &app.RequestContext{}
	c.Request.Header.Set("If-None-Match", `"version-one"`)
	if !contextVideoNotModified(c, "version-one") {
		t.Fatal("matching etag was not handled")
	}
	if c.Response.StatusCode() != http.StatusNotModified {
		t.Fatalf("status=%d", c.Response.StatusCode())
	}
	if got := string(c.Response.Header.Peek("Cache-Control")); got == "" {
		t.Fatal("cache control missing")
	}
}
