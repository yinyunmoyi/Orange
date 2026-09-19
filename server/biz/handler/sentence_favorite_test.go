package handler

import (
	"errors"
	"net/http"
	"testing"

	"aaa_word/biz/db"
	"aaa_word/biz/service"
)

func TestSentenceFavoriteHTTPError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid", err: service.ErrSentenceFavoriteInvalid, wantStatus: http.StatusBadRequest},
		{name: "translation too long", err: service.ErrSentenceTranslationLong, wantStatus: http.StatusBadRequest},
		{name: "not found", err: service.ErrSentenceFavoriteNotFound, wantStatus: http.StatusNotFound},
		{name: "database disabled", err: db.ErrDBDisabled, wantStatus: http.StatusServiceUnavailable},
		{name: "internal", err: errors.New("query failed"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message := sentenceFavoriteHTTPError(tt.err)
			if status != tt.wantStatus {
				t.Fatalf("status=%d, want=%d", status, tt.wantStatus)
			}
			if message == "" {
				t.Fatal("message must not be empty")
			}
		})
	}
}
