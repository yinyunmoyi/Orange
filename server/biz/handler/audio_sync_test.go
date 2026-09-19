package handler

import (
	"errors"
	"net/http"
	"testing"

	"aaa_word/biz/db"
	"aaa_word/biz/service"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestAudioSyncNotModified(t *testing.T) {
	c := &app.RequestContext{}
	c.Request.Header.Set("If-None-Match", `"version-one"`)
	if !audioSyncNotModified(c, "version-one") {
		t.Fatal("matching etag was not handled")
	}
	if c.Response.StatusCode() != http.StatusNotModified {
		t.Fatalf("status=%d", c.Response.StatusCode())
	}
	if got := string(c.Response.Header.Peek("Cache-Control")); got == "" {
		t.Fatal("cache control missing")
	}
}

func TestAudioSyncError(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{service.ErrAudioSyncInvalid, http.StatusBadRequest},
		{service.ErrAudioSyncNotFound, http.StatusNotFound},
		{db.ErrDBDisabled, http.StatusServiceUnavailable},
		{errors.New("unexpected"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		if got, _ := audioSyncError(test.err); got != test.want {
			t.Fatalf("err=%v status=%d want=%d", test.err, got, test.want)
		}
	}
}
