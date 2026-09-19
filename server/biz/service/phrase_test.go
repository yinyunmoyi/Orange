package service

import (
	"errors"
	"testing"
)

func TestNormalizePhrase(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "normalizes case and spaces", raw: "  Look   Up  ", want: "look up"},
		{name: "apostrophe and hyphen", raw: "Mother-in-law's House", want: "mother-in-law's house"},
		{name: "single token", raw: "hello", wantErr: true},
		{name: "too many tokens", raw: "one two three four five six seven eight nine", wantErr: true},
		{name: "invalid character", raw: "look up!", wantErr: true},
		{name: "broken hyphenated word", raw: "on the off-c hance", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePhrase(tt.raw)
			if tt.wantErr {
				if !errors.Is(err, ErrPhraseInvalid) {
					t.Fatalf("expected invalid phrase, got phrase=%q err=%v", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got phrase=%q err=%v, want=%q", got, err, tt.want)
			}
		})
	}
}

func TestLearningItemKeySeparatesTypes(t *testing.T) {
	word := learningKey(LearningItemWord, 7)
	phrase := learningKey(LearningItemPhrase, 7)
	if word == phrase {
		t.Fatalf("word and phrase keys collided: %+v", word)
	}
	items := map[learningItemKey]struct{}{word: {}, phrase: {}}
	if len(items) != 2 {
		t.Fatalf("item count=%d, want=2", len(items))
	}
}
