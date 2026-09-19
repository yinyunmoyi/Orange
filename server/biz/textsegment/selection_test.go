package textsegment

import (
	"errors"
	"testing"
)

func TestSliceUTF16(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		start   int
		end     int
		want    string
		wantErr bool
	}{
		{
			name:  "phrase",
			text:  "Harry asked, looking around at them all.",
			start: 13,
			end:   27,
			want:  "looking around",
		},
		{
			name:  "surrogate before phrase",
			text:  "😀 looking around",
			start: 3,
			end:   17,
			want:  "looking around",
		},
		{
			name:    "range splits surrogate",
			text:    "😀 hello",
			start:   1,
			end:     2,
			wantErr: true,
		},
		{
			name:    "range out of bounds",
			text:    "hello",
			start:   0,
			end:     99,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SliceUTF16(tt.text, tt.start, tt.end)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("SliceUTF16() expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SliceUTF16() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("SliceUTF16() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateSelection(t *testing.T) {
	paragraph := "“What are you talking about?” Harry asked, looking   around at them all."
	start := utf16Index(paragraph, "looking   around")
	end := start + utf16Len("looking   around")

	selected, err := ValidateSelection(paragraph, start, end, "LOOKING AROUND")
	if err != nil {
		t.Fatalf("ValidateSelection() error = %v", err)
	}
	if selected != "looking   around" {
		t.Fatalf("selected = %q", selected)
	}
}

func TestValidateSelectionMismatch(t *testing.T) {
	paragraph := "Harry asked, looking around at them all."
	start := utf16Index(paragraph, "looking around")
	end := start + utf16Len("looking around")

	selected, err := ValidateSelection(paragraph, start, end, "talking about")
	if !errors.Is(err, ErrSelectionMismatch) {
		t.Fatalf("ValidateSelection() error = %v, want ErrSelectionMismatch", err)
	}
	if selected != "looking around" {
		t.Fatalf("selected = %q", selected)
	}
}

func TestUTF16Length(t *testing.T) {
	if got := UTF16Length("😀a"); got != 3 {
		t.Fatalf("UTF16Length() = %d, want 3", got)
	}
}
