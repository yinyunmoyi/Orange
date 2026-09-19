package textsegment

import (
	"errors"
	"strings"
	"unicode/utf16"
)

var ErrSelectionMismatch = errors.New("highlight does not match context item")

// UTF16Length 返回与 JavaScript/Kotlin String 索引一致的 UTF-16 单元长度。
func UTF16Length(text string) int {
	return len(utf16.Encode([]rune(text)))
}

// ValidateUTF16Range 校验 UTF-16 左闭右开区间，且不允许切开代理对。
func ValidateUTF16Range(text string, start, end int) error {
	units := utf16.Encode([]rune(text))
	if start < 0 || end <= start || end > len(units) {
		return errors.New("range out of bounds")
	}
	if !isUTF16Boundary(units, start) || !isUTF16Boundary(units, end) {
		return errors.New("range splits a UTF-16 surrogate pair")
	}
	return nil
}

// SliceUTF16 按 UTF-16 区间提取文本。
func SliceUTF16(text string, start, end int) (string, error) {
	if err := ValidateUTF16Range(text, start, end); err != nil {
		return "", err
	}
	units := utf16.Encode([]rune(text))
	return string(utf16.Decode(units[start:end])), nil
}

// ValidateSelection 校验段落选区与收藏项文本是否一致。
// 比较时忽略大小写，并把连续空白折叠为单个空格。
func ValidateSelection(paragraph string, start, end int, itemText string) (string, error) {
	selected, err := SliceUTF16(paragraph, start, end)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(normalizeSelectionText(selected), normalizeSelectionText(itemText)) {
		return selected, ErrSelectionMismatch
	}
	return selected, nil
}

func normalizeSelectionText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
