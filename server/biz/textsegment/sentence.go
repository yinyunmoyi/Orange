package textsegment

import (
	"errors"
	"strings"
	"unicode/utf16"
)

const (
	// Version 标识当前确定性切句规则版本。
	Version = "rule_en_v3"

	maxSentenceRunes = 500
	maxSpeakerWords  = 3
)

var abbreviations = map[string]struct{}{
	"mr":   {},
	"mrs":  {},
	"ms":   {},
	"dr":   {},
	"prof": {},
	"sr":   {},
	"jr":   {},
	"st":   {},
	"e":    {},
	"g":    {},
	"i":    {},
	"etc":  {},
	"vs":   {},
	"no":   {},
}

var speechTagVerbs = map[string]struct{}{
	"added":     {},
	"answered":  {},
	"asked":     {},
	"called":    {},
	"continued": {},
	"cried":     {},
	"demanded":  {},
	"exclaimed": {},
	"murmured":  {},
	"muttered":  {},
	"remarked":  {},
	"repeated":  {},
	"replied":   {},
	"said":      {},
	"shouted":   {},
	"whispered": {},
	"yelled":    {},
}

// Result 是从原始段落中提取出的句子及其 UTF-16 位置。
type Result struct {
	Sentence       string
	SentenceStart  int
	SentenceEnd    int
	HighlightStart int
	HighlightEnd   int
	Version        string
}

// ExtractSentence 返回包含选区的完整句子。所有输入和输出位置均使用 UTF-16 单元。
func ExtractSentence(paragraph string, selectionStart, selectionEnd int) (Result, error) {
	if paragraph == "" {
		return Result{}, errors.New("paragraph is empty")
	}
	units := utf16.Encode([]rune(paragraph))
	if selectionStart < 0 || selectionEnd <= selectionStart || selectionEnd > len(units) {
		return Result{}, errors.New("selection is out of bounds")
	}
	if !isUTF16Boundary(units, selectionStart) || !isUTF16Boundary(units, selectionEnd) {
		return Result{}, errors.New("selection splits a UTF-16 surrogate pair")
	}

	boundaries := sentenceBoundaries(units)
	for _, boundary := range boundaries {
		if boundary > selectionStart && boundary < selectionEnd {
			return Result{}, errors.New("selection crosses a sentence boundary")
		}
	}

	sentenceStart := 0
	for _, boundary := range boundaries {
		if boundary <= selectionStart {
			sentenceStart = boundary
			continue
		}
		break
	}
	for sentenceStart < len(units) && isWhitespace(units[sentenceStart]) {
		sentenceStart++
	}

	sentenceEnd := len(units)
	for _, boundary := range boundaries {
		if boundary >= selectionEnd {
			sentenceEnd = boundary
			break
		}
	}
	for sentenceEnd > sentenceStart && isWhitespace(units[sentenceEnd-1]) {
		sentenceEnd--
	}

	if sentenceStart >= sentenceEnd || selectionStart < sentenceStart || selectionEnd > sentenceEnd {
		return Result{}, errors.New("no sentence contains the selection")
	}
	sentence := string(utf16.Decode(units[sentenceStart:sentenceEnd]))
	if len([]rune(sentence)) > maxSentenceRunes {
		return Result{}, errors.New("extracted sentence is too long")
	}

	return Result{
		Sentence:       sentence,
		SentenceStart:  sentenceStart,
		SentenceEnd:    sentenceEnd,
		HighlightStart: selectionStart - sentenceStart,
		HighlightEnd:   selectionEnd - sentenceStart,
		Version:        Version,
	}, nil
}

func sentenceBoundaries(units []uint16) []int {
	boundaries := make([]int, 0, 8)
	for i := 0; i < len(units); i++ {
		if !isTerminal(units[i]) {
			continue
		}

		groupEnd := i
		for groupEnd+1 < len(units) && isTerminal(units[groupEnd+1]) {
			groupEnd++
		}
		isGroup := groupEnd > i
		if !isGroup && units[i] == '.' && !isPeriodBoundary(units, i) {
			continue
		}

		end := groupEnd + 1
		for end < len(units) && isClosingPunctuation(units[end]) {
			end++
		}
		hasClosingQuote := containsClosingQuote(units, groupEnd+1, end)
		if end > groupEnd+1 &&
			(continuesWithLowercase(units, end) ||
				hasClosingQuote && continuesWithNamedSpeechTag(units, end)) {
			i = groupEnd
			continue
		}
		boundaries = append(boundaries, end)
		i = groupEnd
	}
	return boundaries
}

func continuesWithLowercase(units []uint16, index int) bool {
	for index < len(units) && isWhitespace(units[index]) {
		index++
	}
	return index < len(units) && units[index] >= 'a' && units[index] <= 'z'
}

func containsClosingQuote(units []uint16, start, end int) bool {
	for i := start; i < end; i++ {
		switch units[i] {
		case '"', '\'', '’', '”', '»':
			return true
		}
	}
	return false
}

// continuesWithNamedSpeechTag 识别 `"..." Harry asked` 形式的后置发言归属语。
// 仅接受至多三个以大写字母开头的说话人词，并要求随后出现明确的发言动词。
func continuesWithNamedSpeechTag(units []uint16, index int) bool {
	index = skipWhitespace(units, index)
	for speakerWords := 0; speakerWords < maxSpeakerWords; speakerWords++ {
		token, next := nextASCIIWord(units, index)
		if token == "" {
			return false
		}
		if _, ok := speechTagVerbs[strings.ToLower(token)]; ok {
			return speakerWords > 0
		}
		if token[0] < 'A' || token[0] > 'Z' {
			return false
		}
		index = skipWhitespace(units, next)
	}
	token, _ := nextASCIIWord(units, index)
	_, ok := speechTagVerbs[strings.ToLower(token)]
	return ok
}

func skipWhitespace(units []uint16, index int) int {
	for index < len(units) && isWhitespace(units[index]) {
		index++
	}
	return index
}

func nextASCIIWord(units []uint16, index int) (string, int) {
	start := index
	for index < len(units) && isASCIIAlpha(units[index]) {
		index++
	}
	if start == index {
		return "", start
	}
	return string(utf16.Decode(units[start:index])), index
}

func isPeriodBoundary(units []uint16, index int) bool {
	var prev, next uint16
	if index > 0 {
		prev = units[index-1]
	}
	if index+1 < len(units) {
		next = units[index+1]
	}

	if isASCIIDigit(prev) && isASCIIDigit(next) {
		return false
	}
	if next != 0 && !isWhitespace(next) && isASCIIAlphaNumeric(next) {
		return false
	}

	token := precedingASCIIWord(units, index)
	if _, ok := abbreviations[strings.ToLower(token)]; ok {
		return false
	}
	if len(token) == 1 && isASCIIAlphaNumeric(uint16(token[0])) {
		return false
	}
	return true
}

func precedingASCIIWord(units []uint16, periodIndex int) string {
	start := periodIndex
	for start > 0 && isASCIIAlpha(units[start-1]) {
		start--
	}
	return string(utf16.Decode(units[start:periodIndex]))
}

func isUTF16Boundary(units []uint16, index int) bool {
	if index <= 0 || index >= len(units) {
		return true
	}
	return !(isHighSurrogate(units[index-1]) && isLowSurrogate(units[index]))
}

func isHighSurrogate(unit uint16) bool {
	return unit >= 0xD800 && unit <= 0xDBFF
}

func isLowSurrogate(unit uint16) bool {
	return unit >= 0xDC00 && unit <= 0xDFFF
}

func isTerminal(unit uint16) bool {
	switch unit {
	case '.', '!', '?', '。', '！', '？':
		return true
	default:
		return false
	}
}

func isClosingPunctuation(unit uint16) bool {
	switch unit {
	case '"', '\'', '’', '”', ')', ']', '}', '»':
		return true
	default:
		return false
	}
}

func isWhitespace(unit uint16) bool {
	switch unit {
	case ' ', '\t', '\n', '\r', '\f', '\v', '\u00A0':
		return true
	default:
		return false
	}
}

func isASCIIAlpha(unit uint16) bool {
	return unit >= 'a' && unit <= 'z' || unit >= 'A' && unit <= 'Z'
}

func isASCIIDigit(unit uint16) bool {
	return unit >= '0' && unit <= '9'
}

func isASCIIAlphaNumeric(unit uint16) bool {
	return isASCIIAlpha(unit) || isASCIIDigit(unit)
}
