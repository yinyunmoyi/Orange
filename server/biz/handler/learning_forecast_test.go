package handler

import "testing"

func TestParseForecastDays(t *testing.T) {
	tests := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{raw: "", want: 14},
		{raw: "1", want: 1},
		{raw: "90", want: 90},
		{raw: "0", wantErr: true},
		{raw: "-1", wantErr: true},
		{raw: "91", wantErr: true},
		{raw: "many", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := parseForecastDays(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got=(%d,%v), want=%d", got, err, tt.want)
			}
		})
	}
}
