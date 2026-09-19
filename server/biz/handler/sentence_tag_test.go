package handler

import (
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"aaa_word/biz/db"
	"aaa_word/biz/service"
)

func TestParseSentenceTagIDs(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []int64
		wantErr bool
	}{
		{name: "empty", raw: "", want: nil},
		{name: "values", raw: "3, 1,3", want: []int64{3, 1}},
		{name: "invalid", raw: "1,nope", wantErr: true},
		{name: "non positive", raw: "0", wantErr: true},
		{
			name:    "too many",
			raw:     buildTagIDCSV(service.MaxSentenceTagsPerItem + 1),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSentenceTagIDs(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestSentenceTagHTTPError(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{err: service.ErrSentenceTagInvalid, want: http.StatusBadRequest},
		{err: service.ErrSentenceTagDuplicate, want: http.StatusBadRequest},
		{err: service.ErrSentenceTagNotFound, want: http.StatusNotFound},
		{err: service.ErrSentenceFavoriteNotFound, want: http.StatusNotFound},
		{err: service.ErrSentenceTagConflict, want: http.StatusConflict},
		{err: db.ErrDBDisabled, want: http.StatusServiceUnavailable},
		{err: errors.New("query failed"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		status, message := sentenceTagHTTPError(tt.err)
		if status != tt.want || message == "" {
			t.Fatalf("err=%v status=%d message=%q want=%d", tt.err, status, message, tt.want)
		}
	}
}

func buildTagIDCSV(count int) string {
	parts := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		parts = append(parts, strconv.Itoa(i))
	}
	return strings.Join(parts, ",")
}
