package dict

import (
	"testing"
)

func TestSplitTranslations(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []struct {
			pos  string
			text string
		}
	}{
		{
			name:  "multi lines with pos",
			input: "n. 学生\nvi. 学习\nvt. 学习; 攻读",
			want: []struct {
				pos  string
				text string
			}{
				{"n.", "学生"},
				{"vi.", "学习"},
				{"vt.", "学习; 攻读"},
			},
		},
		{
			name:  "line without pos",
			input: "重要的\n有意义的",
			want: []struct {
				pos  string
				text string
			}{
				{"", "重要的"},
				{"", "有意义的"},
			},
		},
		{
			name:  "with numbered index",
			input: "1. n. 苹果\n2. v. 应用",
			want: []struct {
				pos  string
				text string
			}{
				{"n.", "苹果"},
				{"v.", "应用"},
			},
		},
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "blank lines",
			input: "\n\nn. 桌子\n\n",
			want: []struct {
				pos  string
				text string
			}{
				{"n.", "桌子"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitTranslations(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch: got %d want %d (%+v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].PartOfSpeech != tc.want[i].pos || got[i].Meaning != tc.want[i].text {
					t.Errorf("row %d: got %+v, want pos=%q text=%q", i, got[i], tc.want[i].pos, tc.want[i].text)
				}
			}
		})
	}
}

func TestSplitDefinitions(t *testing.T) {
	got := SplitDefinitions("a person who studies\na young learner")
	if len(got) != 1 {
		t.Fatalf("expected 1 group, got %d", len(got))
	}
	if len(got[0].Definitions) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(got[0].Definitions))
	}
	if got[0].Definitions[0].Definition != "a person who studies" {
		t.Errorf("unexpected def[0]: %q", got[0].Definitions[0].Definition)
	}
}

func TestSplitDefinitionsWithWordNetPOS(t *testing.T) {
	got := SplitDefinitions("r. in a grudging manner\nn. one who grudges\nv. give reluctantly")
	if len(got) != 3 {
		t.Fatalf("expected 3 groups, got %d (%+v)", len(got), got)
	}
	wants := []struct {
		pos string
		def string
	}{
		{"adv.", "in a grudging manner"},
		{"n.", "one who grudges"},
		{"v.", "give reluctantly"},
	}
	for i, w := range wants {
		if got[i].PartOfSpeech != w.pos {
			t.Errorf("group %d pos: %q, want %q", i, got[i].PartOfSpeech, w.pos)
		}
		if len(got[i].Definitions) != 1 || got[i].Definitions[0].Definition != w.def {
			t.Errorf("group %d def: %+v, want %q", i, got[i].Definitions, w.def)
		}
	}
}

func TestSplitDefinitionsIgnoresWebsterAbbrPrefix(t *testing.T) {
	got := SplitDefinitions("imp. & p. p. of Ding")
	if len(got) != 1 {
		t.Fatalf("expected 1 group, got %d", len(got))
	}
	if got[0].PartOfSpeech != "" {
		t.Errorf("expected no POS, got %q", got[0].PartOfSpeech)
	}
	if len(got[0].Definitions) != 1 {
		t.Fatalf("expected 1 def, got %d", len(got[0].Definitions))
	}
	want := "Ding 的过去式和过去分词"
	if got[0].Definitions[0].Definition != want {
		t.Errorf("def = %q, want %q", got[0].Definitions[0].Definition, want)
	}
}

func TestApplyWebsterReplacements(t *testing.T) {
	cases := map[string]string{
		"p. p. of run":                 "run 的过去分词",
		"pl. of goose":                 "goose 的复数形式",
		"compar. of good":              "good 的比较级",
		"3d pers. sing. pres. of be":   "be 的第三人称单数现在时",
		"imp. &, p. p. of Frown":       "Frown 的过去式和过去分词",
		"imp. / p. p. of Affiance":     "Affiance 的过去式和过去分词",
		"imp. &. p. p. of Recuperate":  "Recuperate 的过去式和过去分词",
		"p. pr. & vb/ n. of Calculate": "Calculate 的现在分词和动名词",
		"past participle of forget":    "forget 的过去分词",
		"-s form of relationship":      "relationship 的第三人称单数形式",
	}
	for in, want := range cases {
		if got := applyWebsterReplacements(in); got != want {
			t.Errorf("applyWebsterReplacements(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitDefinitionsNormalizesWebsterCompoundPOS(t *testing.T) {
	got := SplitDefinitions("v. t. To regard with displeasure.\nv. i. To show displeasure.")
	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d: %+v", len(got), got)
	}
	if got[0].PartOfSpeech != "vt." || got[0].Definitions[0].Definition != "To regard with displeasure." {
		t.Errorf("transitive definition = %+v", got[0])
	}
	if got[1].PartOfSpeech != "vi." || got[1].Definitions[0].Definition != "To show displeasure." {
		t.Errorf("intransitive definition = %+v", got[1])
	}
}

func TestNormalizeDefinitionRepairsPreviouslyStoredRows(t *testing.T) {
	pos, definition := NormalizeDefinition("", "imp. &, p. p. of Frown")
	if pos != "" || definition != "Frown 的过去式和过去分词" {
		t.Fatalf("normalized inflection = (%q, %q)", pos, definition)
	}

	pos, definition = NormalizeDefinition("v.", "t. To knit the brows.")
	if pos != "vt." || definition != "To knit the brows." {
		t.Fatalf("normalized transitive verb = (%q, %q)", pos, definition)
	}

	pos, definition = NormalizeDefinition("", "v throw a glance at; take a brief look at")
	if pos != "v." || definition != "throw a glance at; take a brief look at" {
		t.Fatalf("normalized bare WordNet verb = (%q, %q)", pos, definition)
	}

	pos, definition = NormalizeDefinition("", "a person who looks briefly")
	if pos != "" || definition != "a person who looks briefly" {
		t.Fatalf("article must not be treated as POS = (%q, %q)", pos, definition)
	}
}

func TestWordNetPOSAbbr(t *testing.T) {
	cases := map[string]string{
		"n":    "n.",
		"v":    "v.",
		"a":    "adj.",
		"s":    "adj.",
		"r":    "adv.",
		"":     "",
		"prep": "prep",
	}
	for in, want := range cases {
		if got := WordNetPOSAbbr(in); got != want {
			t.Errorf("WordNetPOSAbbr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEcdictPOSAbbr(t *testing.T) {
	cases := map[string]string{
		"n":  "n.",
		"v":  "v.",
		"j":  "adj.",
		"r":  "adv.",
		"p":  "pron.",
		"c":  "conj.",
		"i":  "prep.",
		"u":  "interj.",
		"d":  "det.",
		"m":  "num.",
		"a":  "art.",
		"t":  "to",
		"":   "",
		"xx": "xx",
	}
	for in, want := range cases {
		if got := EcdictPOSAbbr(in); got != want {
			t.Errorf("EcdictPOSAbbr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizePosField(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"n:100":        "n.:100",
		"j:60/v:40":    "adj.:60/v.:40",
		"r:100":        "adv.:100",
		"i:50/c:50":    "prep.:50/conj.:50",
		"unknown:10/v": "unknown:10/v.",
		"n:80/xx:5":    "n.:80/xx:5",
	}
	for in, want := range cases {
		if got := NormalizePosField(in); got != want {
			t.Errorf("NormalizePosField(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitTags(t *testing.T) {
	got := SplitTags("cet4  cet6\ttoefl gre")
	want := []string{"cet4", "cet6", "toefl", "gre"}
	if len(got) != len(want) {
		t.Fatalf("len %d != %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("tag %d: %q != %q", i, got[i], want[i])
		}
	}
	if SplitTags("") != nil {
		t.Errorf("empty tag should be nil")
	}
}

func TestNormalizePhonetic(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"əˈbændən":   "/əˈbændən/",
		"/əˈbændən/": "/əˈbændən/",
		" əˈbændən ": "/əˈbændən/",
	}
	for in, want := range cases {
		if got := NormalizePhonetic(in); got != want {
			t.Errorf("NormalizePhonetic(%q) = %q, want %q", in, got, want)
		}
	}
}
