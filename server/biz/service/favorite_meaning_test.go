package service

import (
	"testing"

	"aaa_word/biz/db"
)

func TestGroupEnMeaningsNormalizesCachedWebsterDefinitions(t *testing.T) {
	got := groupEnMeanings([]db.WordMeaning{
		{PartOfSpeech: "", Definition: "imp. &, p. p. of Frown"},
		{PartOfSpeech: "v.", Definition: "t. To knit the brows."},
	})
	if len(got) != 2 {
		t.Fatalf("meaning groups = %d, want 2: %+v", len(got), got)
	}
	if got[0].Definitions[0].Definition != "Frown 的过去式和过去分词" {
		t.Fatalf("cached inflection = %q", got[0].Definitions[0].Definition)
	}
	if got[1].PartOfSpeech != "vt." || got[1].Definitions[0].Definition != "To knit the brows." {
		t.Fatalf("cached transitive definition = %+v", got[1])
	}
}
