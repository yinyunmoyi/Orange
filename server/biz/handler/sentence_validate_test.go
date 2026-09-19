package handler

import "testing"

// TestSentencePatternAcceptsCommonLatinChars 覆盖 bad case：句中含 é/à/ü/ñ 等
// 常见拉丁扩展字母时，AnalyzeSentence / TranslateSentence 应通过校验。
// 触发场景：Tomohiko looked around the café...
func TestSentencePatternAcceptsCommonLatinChars(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool // want match after normalize
	}{
		{
			name:  "café with acute e",
			input: "Tomohiko looked around the café. Usually places like this had a Space Invaders game in the corner, but there was nothing of the kind here.",
			want:  true,
		},
		{name: "plain ascii", input: "Hello, world!", want: true},
		{name: "curly quotes normalized", input: "He said \u201Chello\u201D to me.", want: true},
		{name: "em dash normalized", input: "It was raining\u2014again.", want: true},
		{name: "naive with diaeresis", input: "That was a naïve assumption.", want: true},
		{name: "resume with accents", input: "Please send your résumé.", want: true},
		{name: "chinese should be rejected", input: "你好 world", want: false},
		{name: "emoji should be rejected", input: "Hello 😀", want: false},
		{name: "empty after trim", input: "   ", want: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := normalizeSentence(tt.input)
			if s == "" {
				if tt.want {
					t.Fatalf("normalize produced empty for %q, want match", tt.input)
				}
				return
			}
			got := sentencePattern.MatchString(s)
			if got != tt.want {
				t.Fatalf("pattern match=%v, want=%v (normalized=%q)", got, tt.want, s)
			}
		})
	}
}
