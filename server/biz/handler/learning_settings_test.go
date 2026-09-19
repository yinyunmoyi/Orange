package handler

import (
	"errors"
	"net/http"
	"testing"

	"aaa_word/biz/db"
	"aaa_word/biz/service"
)

func TestLearningSettingsHTTPError(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
	}{
		{err: service.ErrLearningSettingsInvalid, wantStatus: http.StatusBadRequest},
		{err: db.ErrDBDisabled, wantStatus: http.StatusServiceUnavailable},
		{err: errors.New("query failed"), wantStatus: http.StatusInternalServerError},
	}
	for _, test := range tests {
		status, message := learningSettingsHTTPError(test.err)
		if status != test.wantStatus || message == "" {
			t.Fatalf("error=%v got=(%d,%q), want status=%d",
				test.err, status, message, test.wantStatus)
		}
	}
}
