package dict

import (
	"regexp"
	"strings"

	"aaa_word/biz/model"
)

// wordNetPOSCodes 是 ECDICT 英文释义行首合法的 WordNet 词性标记集合。
// 只有落在这个集合内的前缀才会被抽出，避免把 "imp."（imperfect）等旧词典缩写误判为 POS。
var wordNetPOSCodes = map[string]struct{}{
	"n": {}, "v": {}, "a": {}, "s": {}, "r": {},
}

// posPrefixRe 匹配义项行首的词性缩写，如 "n. 学生"、"v. 抛弃 (曾经)"、"adj. 空的"。
// 允许字母后跟点号，再跟一到多个空白。
var posPrefixRe = regexp.MustCompile(`^([a-zA-Z]{1,6})\.\s+(.+)$`)

// bareWordNetPOSPrefixRe 兼容部分数据源省略句点的 WordNet 标记，如 "v look briefly"。
// 不包含容易与英文冠词混淆的 "a"，避免把 "a person ..." 误判为形容词释义。
var bareWordNetPOSPrefixRe = regexp.MustCompile(`(?i)^([nvrs])\s+(.+)$`)

var websterCompoundPOSPrefixes = []struct {
	pattern *regexp.Regexp
	pos     string
}{
	{regexp.MustCompile(`(?i)^v\.\s*t\.\s*(?:&|and)\s*i\.\s+`), "v."},
	{regexp.MustCompile(`(?i)^v\.\s*i\.\s*(?:&|and)\s*t\.\s+`), "v."},
	{regexp.MustCompile(`(?i)^v\.\s*t\.\s+`), "vt."},
	{regexp.MustCompile(`(?i)^v\.\s*i\.\s+`), "vi."},
	{regexp.MustCompile(`(?i)^p\.\s*a\.\s+`), "adj."},
}

var (
	storedTransitivePrefix   = regexp.MustCompile(`(?i)^t\.\s+`)
	storedIntransitivePrefix = regexp.MustCompile(`(?i)^i\.\s+`)
)

const inflectionTargetPattern = `([A-Za-z][A-Za-z'-]*(?:\s+[A-Za-z][A-Za-z'-]*){0,3})`

// websterAbbrReplacements 将整条词形说明转换为中文，规则均锚定行首，避免在普通释义中误替换。
// 分隔符允许 Webster 数据中真实存在的 &,、&.、/、漏空格和漏点号变体。
var websterAbbrReplacements = []struct {
	pattern *regexp.Regexp
	repl    string
}{
	{regexp.MustCompile(`(?i)^\s*3d\s+pers\.\s+sing\.\s+pres\.\s*of\s+` + inflectionTargetPattern), "$1 的第三人称单数现在时"},
	{regexp.MustCompile(`(?i)^\s*2d\s+pers\.\s+sing\.\s+pres\.\s*of\s+` + inflectionTargetPattern), "$1 的第二人称单数现在时"},
	{regexp.MustCompile(`(?i)^\s*imp\.\s*[&/.,\s]*p\.\s*p\.?\s*[&/.,\s]*(?:v+b|v)\s*[./,]?\s*n\.?\s+of\s+` + inflectionTargetPattern), "$1 的过去式、过去分词和动名词"},
	{regexp.MustCompile(`(?i)^\s*imp\.\s*[&/.,\s]*p\.\s*p\.?\s+of\s+` + inflectionTargetPattern), "$1 的过去式和过去分词"},
	{regexp.MustCompile(`(?i)^\s*imp\.\s*[&/.,\s]*p\.\s*pr\.?\s*[&/.,\s]*(?:v+b|v)\.?\s+of\s+` + inflectionTargetPattern), "$1 的过去式、现在分词和动名词"},
	{regexp.MustCompile(`(?i)^\s*imp\.\s*[&/.,\s]*p\.\s*pr\.?\s+of\s+` + inflectionTargetPattern), "$1 的过去式和现在分词"},
	{regexp.MustCompile(`(?i)^\s*p\.\s*pr\.\s*[&/.,\s]*(?:pr\.\s*[&/.,\s]*)?(?:v+b|v)\s*[./,]?\s*n?\.?\s+of\s+` + inflectionTargetPattern), "$1 的现在分词和动名词"},
	{regexp.MustCompile(`(?i)^\s*p\.\s*p\.\s*[&/.,\s]*p\.\s*a\.\s+of\s+` + inflectionTargetPattern), "$1 的过去分词和分词形容词"},
	{regexp.MustCompile(`(?i)^\s*p\.\s*p\.?\s*[&/.,\s]*of\s+` + inflectionTargetPattern), "$1 的过去分词"},
	{regexp.MustCompile(`(?i)^\s*p\.\s*pr\.?\s+of\s+` + inflectionTargetPattern), "$1 的现在分词"},
	{regexp.MustCompile(`(?i)^\s*vb[./,]?\s*n\.?\s+of\s+` + inflectionTargetPattern), "$1 的动名词"},
	{regexp.MustCompile(`(?i)^\s*(?:pres\.|present tense)\s+of\s+` + inflectionTargetPattern), "$1 的现在时"},
	{regexp.MustCompile(`(?i)^\s*(?:imp\.|past tense)\s+of\s+` + inflectionTargetPattern), "$1 的过去式"},
	{regexp.MustCompile(`(?i)^\s*(?:p\.\s*p\.|past participle)\s+of\s+` + inflectionTargetPattern), "$1 的过去分词"},
	{regexp.MustCompile(`(?i)^\s*(?:pl\.|plural)\s+of\s+` + inflectionTargetPattern), "$1 的复数形式"},
	{regexp.MustCompile(`(?i)^\s*(?:sing\.|singular)\s+of\s+` + inflectionTargetPattern), "$1 的单数形式"},
	{regexp.MustCompile(`(?i)^\s*(?:compar\.|comparative)\s+of\s+` + inflectionTargetPattern), "$1 的比较级"},
	{regexp.MustCompile(`(?i)^\s*(?:superl\.|superlative)\s+of\s+` + inflectionTargetPattern), "$1 的最高级"},
	{regexp.MustCompile(`(?i)^\s*fem\.\s+of\s+` + inflectionTargetPattern), "$1 的阴性形式"},
	{regexp.MustCompile(`(?i)^\s*masc\.\s+of\s+` + inflectionTargetPattern), "$1 的阳性形式"},
	{regexp.MustCompile(`(?i)^\s*-ing\s+form\s+of\s+` + inflectionTargetPattern), "$1 的 -ing 形式"},
	{regexp.MustCompile(`(?i)^\s*-s\s+form\s+of\s+` + inflectionTargetPattern), "$1 的第三人称单数形式"},
}

// applyWebsterReplacements 把常见的 Webster 缩写替换成人话，不影响未匹配到的行。
func applyWebsterReplacements(s string) string {
	for _, r := range websterAbbrReplacements {
		if r.pattern.MatchString(s) {
			return strings.TrimSpace(r.pattern.ReplaceAllString(s, r.repl))
		}
	}
	return s
}

func splitDefinitionPOS(line string) (string, string) {
	for _, prefix := range websterCompoundPOSPrefixes {
		if prefix.pattern.MatchString(line) {
			return prefix.pos, strings.TrimSpace(prefix.pattern.ReplaceAllString(line, ""))
		}
	}
	if m := posPrefixRe.FindStringSubmatch(line); m != nil {
		code := strings.ToLower(m[1])
		if pos := definitionPOSAbbr(code); pos != "" {
			return pos, strings.TrimSpace(m[2])
		}
	}
	if m := bareWordNetPOSPrefixRe.FindStringSubmatch(line); m != nil {
		if pos := definitionPOSAbbr(m[1]); pos != "" {
			return pos, strings.TrimSpace(m[2])
		}
	}
	return "", line
}

// NormalizeDefinition 同时兼容 ECDICT 原始行和收藏后拆分保存的旧数据。
// 旧版本可能已把 "v. t." 拆成 pos="v."、definition="t. ..."，这里一并修复。
func NormalizeDefinition(partOfSpeech, definition string) (string, string) {
	pos := strings.TrimSpace(partOfSpeech)
	line := strings.TrimSpace(definition)
	if normalized := definitionPOSAbbr(strings.TrimSuffix(pos, ".")); normalized != "" {
		pos = normalized
	}
	if pos == "v." {
		switch {
		case storedTransitivePrefix.MatchString(line):
			pos = "vt."
			line = storedTransitivePrefix.ReplaceAllString(line, "")
		case storedIntransitivePrefix.MatchString(line):
			pos = "vi."
			line = storedIntransitivePrefix.ReplaceAllString(line, "")
		}
	}
	if pos == "" {
		pos, line = splitDefinitionPOS(line)
	}
	return pos, applyWebsterReplacements(strings.TrimSpace(line))
}

func definitionPOSAbbr(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if _, ok := wordNetPOSCodes[code]; ok {
		return WordNetPOSAbbr(code)
	}
	switch code {
	case "adj":
		return "adj."
	case "adv":
		return "adv."
	case "vt":
		return "vt."
	case "vi":
		return "vi."
	case "prep":
		return "prep."
	case "pron":
		return "pron."
	case "conj":
		return "conj."
	case "interj":
		return "interj."
	case "num":
		return "num."
	default:
		return ""
	}
}

// leadingIndexRe 去除行首形如 "1. " / "1) " / "(1) " 的编号。
var leadingIndexRe = regexp.MustCompile(`^(?:\(\d+\)|\d+[.)])\s*`)

// SplitTranslations 把 ECDICT translation 字段拆成结构化的中文释义列表。
//
// 输入示例：
//
//	"n. 学生\nvi. 学习\nvt. 学习; 攻读"
//
// 输出按原顺序拆条；若行首带有 POS 缩写则抽出至 PartOfSpeech，否则整行入 Meaning。
func SplitTranslations(translation string) []model.ChineseMeaning {
	if strings.TrimSpace(translation) == "" {
		return nil
	}
	lines := splitLines(translation)
	result := make([]model.ChineseMeaning, 0, len(lines))
	for _, line := range lines {
		line = leadingIndexRe.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := posPrefixRe.FindStringSubmatch(line); m != nil {
			result = append(result, model.ChineseMeaning{
				PartOfSpeech: m[1] + ".",
				Meaning:      strings.TrimSpace(m[2]),
			})
			continue
		}
		result = append(result, model.ChineseMeaning{Meaning: line})
	}
	return result
}

// SplitDefinitions 把 ECDICT definition 字段拆成 model.Meaning 结构。
// 只抽取已知词性前缀（其余如 "imp." 交给词形说明规则处理）。
// 同时对旧 Webster 缩写做人性化替换，帮助用户读懂。
func SplitDefinitions(definition string) []model.Meaning {
	if strings.TrimSpace(definition) == "" {
		return nil
	}
	lines := splitLines(definition)
	grouped := make(map[string][]model.Definition, 4)
	order := make([]string, 0, 4)
	for _, line := range lines {
		line = leadingIndexRe.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pos, definitionLine := NormalizeDefinition("", line)
		line = definitionLine
		if _, ok := grouped[pos]; !ok {
			order = append(order, pos)
		}
		grouped[pos] = append(grouped[pos], model.Definition{Definition: line})
	}
	if len(order) == 0 {
		return nil
	}
	result := make([]model.Meaning, 0, len(order))
	for _, pos := range order {
		result = append(result, model.Meaning{PartOfSpeech: pos, Definitions: grouped[pos]})
	}
	return result
}

// WordNetPOSAbbr 把 ECDICT/WordNet 里的单字母词性缩写规范化成常见形式，
// 未识别的编码原样返回，避免误伤形如 "adv"/"prep" 等已经规范的输入。
func WordNetPOSAbbr(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "n":
		return "n."
	case "v":
		return "v."
	case "a", "s":
		return "adj."
	case "r":
		return "adv."
	case "":
		return ""
	default:
		return code
	}
}

// EcdictPOSAbbr 把 ECDICT stardict.pos 字段里的单字母编码规范化成人类可读形式。
// ECDICT pos 与 WordNet 定义不同：a=冠词、j=形容词，需单独维护。
func EcdictPOSAbbr(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "n":
		return "n."
	case "v":
		return "v."
	case "j":
		return "adj."
	case "r":
		return "adv."
	case "p":
		return "pron."
	case "c":
		return "conj."
	case "i":
		return "prep."
	case "u":
		return "interj."
	case "d":
		return "det."
	case "m":
		return "num."
	case "a":
		return "art."
	case "t":
		return "to"
	case "":
		return ""
	default:
		return code
	}
}

// NormalizePosField 把 ECDICT `pos` 分布字段（如 "j:35/v:20"）中的字母编码
// 替换为可读缩写（"adj.:35/v.:20"），保持 "code:percent" 与 '/' 分隔的格式。
// 未识别的字母原样保留。空字符串直接返回。
func NormalizePosField(pos string) string {
	if strings.TrimSpace(pos) == "" {
		return ""
	}
	segs := strings.Split(pos, "/")
	out := make([]string, 0, len(segs))
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		code, rest, hasRest := strings.Cut(seg, ":")
		abbr := EcdictPOSAbbr(strings.TrimSpace(code))
		if abbr == "" {
			abbr = strings.TrimSpace(code)
		}
		if hasRest {
			out = append(out, abbr+":"+rest)
		} else {
			out = append(out, abbr)
		}
	}
	return strings.Join(out, "/")
}

// SplitTags 把 "cet4 cet6 toefl ielts" 拆成 ["cet4","cet6","toefl","ielts"]。
func SplitTags(tag string) []string {
	fields := strings.Fields(tag)
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// NormalizePhonetic 补齐音标外围的斜杠，保持与前端展示体验一致。
func NormalizePhonetic(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "/") && strings.HasSuffix(p, "/") {
		return p
	}
	return "/" + strings.Trim(p, "/") + "/"
}

// splitLines 按 \n / \r\n 切分并去除空行。
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}
