package handler

import (
	"errors"
	"net/http"
	"testing"

	"aaa_word/biz/db"
	"aaa_word/biz/service"
)

func TestNoteHTTPError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid", err: service.ErrNoteInvalid, wantStatus: http.StatusBadRequest},
		{name: "too long", err: service.ErrNoteTooLong, wantStatus: http.StatusBadRequest},
		{name: "db disabled", err: db.ErrDBDisabled, wantStatus: http.StatusServiceUnavailable},
		{name: "internal", err: errors.New("query failed"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message := noteHTTPError(tt.err)
			if status != tt.wantStatus {
				t.Fatalf("status=%d, want=%d", status, tt.wantStatus)
			}
			if message == "" {
				t.Fatal("message must not be empty")
			}
		})
	}
}
