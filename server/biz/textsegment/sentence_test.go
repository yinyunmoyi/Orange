package textsegment

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestExtractSentence(t *testing.T) {
	tests := []struct {
		name      string
		paragraph string
		selected  string
		want      string
	}{
		{
			name:      "basic sentence",
			paragraph: "First sentence. The storm arrived. Last sentence.",
			selected:  "storm",
			want:      "The storm arrived.",
		},
		{
			name:      "title abbreviation with space",
			paragraph: "I met Mr. Potter yesterday. He waved.",
			selected:  "Potter",
			want:      "I met Mr. Potter yesterday.",
		},
		{
			name:      "title abbreviation without space",
			paragraph: "Mr.Potter entered the room. Everyone stared.",
			selected:  "Potter",
			want:      "Mr.Potter entered the room.",
		},
		{
			name:      "doctor title",
			paragraph: "Dr. Smith arrived early. We began.",
			selected:  "Smith",
			want:      "Dr. Smith arrived early.",
		},
		{
			name:      "common abbreviation",
			paragraph: "Use a tool, e.g. a hammer, carefully. Then stop.",
			selected:  "hammer",
			want:      "Use a tool, e.g. a hammer, carefully.",
		},
		{
			name:      "dotted acronym",
			paragraph: "The U.S. team won. Fans cheered.",
			selected:  "team",
			want:      "The U.S. team won.",
		},
		{
			name:      "initials",
			paragraph: "J. K. Rowling wrote the book. It sold well.",
			selected:  "Rowling",
			want:      "J. K. Rowling wrote the book.",
		},
		{
			name:      "decimal",
			paragraph: "The value was 3.14 meters. It was precise.",
			selected:  "meters",
			want:      "The value was 3.14 meters.",
		},
		{
			name:      "domain",
			paragraph: "Visit example.com for details. Then return.",
			selected:  "details",
			want:      "Visit example.com for details.",
		},
		{
			name:      "quoted sentence",
			paragraph: "He shouted, \"Run now!\" Then he stopped.",
			selected:  "Run",
			want:      "He shouted, \"Run now!\"",
		},
		{
			name:      "quoted question followed by speech tag",
			paragraph: "“Couldn’t have been me, could it?” said Harry sarcastically.",
			selected:  "sarcastically",
			want:      "“Couldn’t have been me, could it?” said Harry sarcastically.",
		},
		{
			name:      "quoted question followed by named speaker tag",
			paragraph: "“What are you talking about?” Harry asked, looking around at them all. They were all regarding him warily.",
			selected:  "looking around",
			want:      "“What are you talking about?” Harry asked, looking around at them all.",
		},
		{
			name:      "parenthesized sentence",
			paragraph: "(Really?) She looked surprised.",
			selected:  "Really",
			want:      "(Really?)",
		},
		{
			name:      "ellipsis",
			paragraph: "Wait... The storm is coming. Go inside.",
			selected:  "storm",
			want:      "The storm is coming.",
		},
		{
			name:      "no terminal",
			paragraph: "A paragraph containing the storm without punctuation",
			selected:  "storm",
			want:      "A paragraph containing the storm without punctuation",
		},
		{
			name:      "utf16 surrogate before selection",
			paragraph: "😀 Intro. The storm came quickly. Done.",
			selected:  "storm",
			want:      "The storm came quickly.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := utf16Index(tt.paragraph, tt.selected)
			if start < 0 {
				t.Fatalf("selected text %q not found", tt.selected)
			}
			end := start + utf16Len(tt.selected)
			got, err := ExtractSentence(tt.paragraph, start, end)
			if err != nil {
				t.Fatalf("ExtractSentence() error = %v", err)
			}
			if got.Version != Version {
				t.Fatalf("version = %q, want %q", got.Version, Version)
			}
			if got.Sentence != tt.want {
				t.Fatalf("sentence = %q, want %q", got.Sentence, tt.want)
			}
			if got.HighlightStart < 0 || got.HighlightEnd > utf16Len(got.Sentence) {
				t.Fatalf("invalid highlight range [%d,%d)", got.HighlightStart, got.HighlightEnd)
			}
			if selected := utf16SliceForTest(got.Sentence, got.HighlightStart, got.HighlightEnd); selected != tt.selected {
				t.Fatalf("highlight = %q, want %q", selected, tt.selected)
			}
			if got.SentenceStart+got.HighlightStart != start || got.SentenceStart+got.HighlightEnd != end {
				t.Fatalf("offsets do not map to original selection: result=%+v selection=[%d,%d)", got, start, end)
			}
		})
	}
}

func TestExtractSentenceErrors(t *testing.T) {
	longSentence := strings.Repeat("a", 501)
	tests := []struct {
		name      string
		paragraph string
		start     int
		end       int
	}{
		{name: "empty paragraph", paragraph: "", start: 0, end: 1},
		{name: "negative start", paragraph: "Hello.", start: -1, end: 2},
		{name: "end out of bounds", paragraph: "Hello.", start: 0, end: 99},
		{name: "empty selection", paragraph: "Hello.", start: 1, end: 1},
		{name: "selection crosses sentence", paragraph: "One. Two.", start: 1, end: 7},
		{name: "sentence too long", paragraph: longSentence, start: 10, end: 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ExtractSentence(tt.paragraph, tt.start, tt.end); err == nil {
				t.Fatal("ExtractSentence() expected error")
			}
		})
	}
}

func utf16Index(s, sub string) int {
	byteIndex := strings.Index(s, sub)
	if byteIndex < 0 {
		return -1
	}
	return utf16Len(s[:byteIndex])
}

func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

func utf16SliceForTest(s string, start, end int) string {
	units := utf16.Encode([]rune(s))
	return string(utf16.Decode(units[start:end]))
}
