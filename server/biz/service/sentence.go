package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"aaa_word/biz/config"
	"aaa_word/biz/model"
)

// ErrLLMBadOutput LLM 输出格式不符合预期，无法解析或字段缺失
var ErrLLMBadOutput = errors.New("llm returned invalid output")

// allowedChunkTypes chunk.type 枚举白名单；不在其中一律回退 other
var allowedChunkTypes = map[string]struct{}{
	"main":          {},
	"modifier":      {},
	"adverbial":     {},
	"parenthetical": {},
	"participle":    {},
	"quote":         {},
	"other":         {},
}

// sentenceSystemPrompt 长难句结构块解析提示词（只做结构分析，翻译由另一个接口负责）
const sentenceSystemPrompt = `你是一名英语句子结构分析器。用户会给你一句英语句子，你需要返回严格 JSON，仅包含句子结构信息，不要输出整句翻译。
只输出严格 JSON，不要输出任何多余文字，不要输出 markdown 代码块，不要解释。
输出字段：
structure: 用简短中文描述句子的主干与从属结构（不超过30字），例如 "主句引语+插入语+解释"。
chunks: 按原句顺序切分出的若干连续文本片段，粗粒度切分（5-10块为宜，不要切太碎）。
chunks 必须是数组。每个 chunk 必须包含以下字段：
text: 原句中的连续子串，保留原大小写、空格和标点，不要改写。
type: 该片段的结构类型（枚举见下）。
role: 该片段作用，用2-6个字概括，如"主句"、"定语"、"引语内容"。
translation: 该片段的中文意思，尽量简短（2-10字），非常简单的片段可留空字符串。
chunks 的切分原则：
按便于理解的结构块切分，不要按单词切分。每个块尽量覆盖较多内容。
每个 chunk 必须是原句中的连续文本。
所有 chunk 的 text 按顺序拼接后，必须完全等于原句（包括空格和标点）。
优先切分出这些结构：主句主干、状语块、插入语、非谓语结构、直接引语。
type 的取值只允许以下几类之一：main、modifier、adverbial、parenthetical、participle、quote、other。
如果某一片段难以精确归类，使用最接近的类型，不要输出空值，不要新增枚举值。
输出格式如下：
{"structure":"...","chunks":[{"text":"...","type":"main","role":"主句","translation":"..."}]}`

// llmSentenceResult 模型返回的原始结构（结构分析接口）
type llmSentenceResult struct {
	Structure string                `json:"structure"`
	Chunks    []model.SentenceChunk `json:"chunks"`
}

// AnalyzeSentence 调用 DeepSeek，返回句子的翻译 + 结构块
func AnalyzeSentence(sentence string) (*model.SentenceAnalysis, error) {
	t0 := time.Now()
	apiKey := config.LLMAPIKey()
	if apiKey == "" {
		log.Printf("[sentence] api key not configured sentence=%q", truncate(sentence, 200))
		return nil, ErrLLMNotConfigured
	}
	log.Printf("[sentence] request sentence=%q", truncate(sentence, 300))

	reqBody := dsRequest{
		Model: config.LLMModel(),
		Messages: []dsMessage{
			{Role: "system", Content: sentenceSystemPrompt},
			{Role: "user", Content: fmt.Sprintf("句子：%s", sentence)},
		},
		Temperature:    0.2,
		MaxTokens:      8192,
		Stream:         false,
		ResponseFormat: &dsRespFormat{Type: "json_object"},
		Thinking:       &dsThinkingConfig{Type: "disabled"},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[sentence] marshal req failed sentence=%q err=%v",
			truncate(sentence, 200), err)
		return nil, fmt.Errorf("marshal llm request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, config.LLMAPIURL(), bytes.NewReader(payload))
	if err != nil {
		log.Printf("[sentence] build req failed sentence=%q err=%v",
			truncate(sentence, 200), err)
		return nil, fmt.Errorf("build llm request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := doLLMRequest(httpReq)
	if err != nil {
		log.Printf("[sentence] call deepseek failed sentence=%q err=%v elapsed=%s",
			truncate(sentence, 200), err, time.Since(t0))
		return nil, fmt.Errorf("call deepseek: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		log.Printf("[sentence] deepseek non-200 sentence=%q status=%d body=%q elapsed=%s",
			truncate(sentence, 200), resp.StatusCode, string(body), time.Since(t0))
		return nil, fmt.Errorf("deepseek status %d", resp.StatusCode)
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[sentence] read resp body failed sentence=%q err=%v elapsed=%s",
			truncate(sentence, 200), err, time.Since(t0))
		return nil, fmt.Errorf("read deepseek response: %w", err)
	}

	var dsResp dsResponse
	if err := json.Unmarshal(rawBody, &dsResp); err != nil {
		log.Printf("[sentence] decode outer response failed sentence=%q err=%v body=%q elapsed=%s",
			truncate(sentence, 200), err, truncate(string(rawBody), 500), time.Since(t0))
		return nil, fmt.Errorf("decode deepseek response: %w", err)
	}
	if len(dsResp.Choices) == 0 {
		log.Printf("[sentence] deepseek returned no choices sentence=%q body=%q usage=%+v elapsed=%s",
			truncate(sentence, 200), truncate(string(rawBody), 500), dsResp.Usage, time.Since(t0))
		return nil, fmt.Errorf("deepseek returned no choices")
	}

	rawContent := dsResp.Choices[0].Message.Content
	reasoningContent := dsResp.Choices[0].Message.ReasoningContent
	finishReason := dsResp.Choices[0].FinishReason
	if strings.TrimSpace(rawContent) == "" {
		log.Printf("[sentence] empty content sentence=%q finish_reason=%s usage=%+v reasoning=%q elapsed=%s",
			truncate(sentence, 200), finishReason, dsResp.Usage, truncate(reasoningContent, 300), time.Since(t0))
		return nil, fmt.Errorf("%w: empty content (finish_reason=%s)", ErrLLMBadOutput, finishReason)
	}
	content := extractJSON(rawContent)
	var raw llmSentenceResult
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		if finishReason == "length" {
			if salvaged, ok := salvageTruncatedSentenceResult(rawContent); ok {
				log.Printf("[sentence] salvaged truncated json sentence=%q salvaged_chunks=%d structure_len=%d usage=%+v elapsed=%s",
					truncate(sentence, 200), len(salvaged.Chunks), len(salvaged.Structure), dsResp.Usage, time.Since(t0))
				raw = salvaged
				goto parsed
			}
		}
		log.Printf("[sentence] parse content as json failed sentence=%q err=%v finish_reason=%s usage=%+v content=%q elapsed=%s",
			truncate(sentence, 200), err, finishReason, dsResp.Usage, truncate(rawContent, 500), time.Since(t0))
		return nil, fmt.Errorf("%w: %v", ErrLLMBadOutput, err)
	}
parsed:

	if strings.TrimSpace(raw.Structure) == "" && len(raw.Chunks) == 0 {
		log.Printf("[sentence] bad output: both empty sentence=%q structure=%q content=%q elapsed=%s",
			truncate(sentence, 200), truncate(raw.Structure, 200), truncate(rawContent, 500), time.Since(t0))
		return nil, ErrLLMBadOutput
	}
	if len(raw.Chunks) == 0 {
		log.Printf("[sentence] bad output: chunks empty sentence=%q structure=%q content=%q elapsed=%s",
			truncate(sentence, 200), truncate(raw.Structure, 200), truncate(rawContent, 500), time.Since(t0))
		return nil, ErrLLMBadOutput
	}

	// 归一化 type / trim role / translation
	for i := range raw.Chunks {
		c := &raw.Chunks[i]
		c.Type = strings.ToLower(strings.TrimSpace(c.Type))
		if _, ok := allowedChunkTypes[c.Type]; !ok {
			if c.Type != "" {
				log.Printf("[sentence] unknown chunk.type=%q fallback=other text=%q",
					c.Type, truncate(c.Text, 80))
			}
			c.Type = "other"
		}
		c.Role = strings.TrimSpace(c.Role)
		c.Translation = strings.TrimSpace(c.Translation)
	}

	// 用后端 diff 兜底，把 chunks 强制对齐到原句
	aligned, report, err := alignChunks(sentence, raw.Chunks)
	if err != nil {
		log.Printf("[sentence] align failed sentence=%q raw_chunks=%d err=%v elapsed=%s",
			truncate(sentence, 200), len(raw.Chunks), err, time.Since(t0))
		return nil, fmt.Errorf("%w: %v", ErrLLMBadOutput, err)
	}
	logAlignReport(sentence, raw.Chunks, aligned, report)

	log.Printf("[sentence] ok sentence=%q structure=%q chunk_count=%d finish_reason=%s usage=%+v elapsed=%s",
		truncate(sentence, 200), truncate(strings.TrimSpace(raw.Structure), 200),
		len(aligned), finishReason, dsResp.Usage, time.Since(t0))

	return &model.SentenceAnalysis{
		Sentence:  sentence,
		Structure: strings.TrimSpace(raw.Structure),
		Chunks:    aligned,
	}, nil
}

// ===== chunks 对齐兜底 =====

// alignReport 记录对齐过程中的差异，用于日志与统计
type alignReport struct {
	InChunks      int  // 模型返回的 chunk 数
	OutChunks     int  // 对齐后 chunk 数
	InsertedFills int  // 插入的补齐 chunk 数（原句里模型漏切的段落）
	TrimmedChars  int  // 被裁掉的模型幻觉字符数（模型输出多出来的字符）
	AdjustedText  int  // text 因空白/大小写与原句不一致被修正的 chunk 数
	Perfect       bool // 是否零差异
}

// alignChunks 用双指针把模型输出的 chunks 定位到原句中，并强制其 text 使用原句原文
// 返回值：对齐后的 chunks、diff 报告、错误
// 错误只在完全无法推进时返回；其它场景通过报告体现
func alignChunks(sentence string, in []model.SentenceChunk) ([]model.SentenceChunk, alignReport, error) {
	report := alignReport{InChunks: len(in)}
	if len(in) == 0 {
		return nil, report, errors.New("no chunks")
	}

	out := make([]model.SentenceChunk, 0, len(in)+2)
	cursor := 0 // 原句里下一个未消费的位置

	for i := range in {
		if cursor >= len(sentence) {
			// 原句已消费完，剩余的模型 chunk 全被判为幻觉
			report.TrimmedChars += len(in[i].Text)
			continue
		}
		start, end, matched := locateChunk(sentence, cursor, in[i].Text)
		if !matched {
			// 该 chunk 完全无法定位；跳过它，计幻觉字符数
			report.TrimmedChars += len(in[i].Text)
			continue
		}

		// 该 chunk 之前有 gap
		if start > cursor {
			gap := sentence[cursor:start]
			if strings.TrimSpace(gap) == "" {
				// 纯空白 gap：直接吸收到当前 chunk 的前面，不算作差异
				start = cursor
			} else {
				// 有内容的 gap：模型漏切了一段，插入补齐 chunk
				out = append(out, model.SentenceChunk{
					Text:        gap,
					Type:        "other",
					Role:        "补齐",
					Translation: "",
				})
				report.InsertedFills++
			}
		}

		aligned := in[i]
		aligned.Text = sentence[start:end]
		out = append(out, aligned)
		if sentence[start:end] != in[i].Text {
			report.AdjustedText++
		}
		cursor = end
	}

	// 尾部还剩内容
	if cursor < len(sentence) {
		tail := sentence[cursor:]
		if strings.TrimSpace(tail) == "" {
			// 纯空白尾巴：并入最后一个 chunk
			if len(out) > 0 {
				out[len(out)-1].Text += tail
			}
		} else {
			out = append(out, model.SentenceChunk{
				Text:        tail,
				Type:        "other",
				Role:        "补齐",
				Translation: "",
			})
			report.InsertedFills++
		}
	}

	report.OutChunks = len(out)
	report.Perfect = report.InsertedFills == 0 && report.TrimmedChars == 0 && report.AdjustedText == 0

	// 兜底 sanity check：拼接后必须等于原句；不等则视为对齐失败
	joined := strings.Builder{}
	for _, c := range out {
		joined.WriteString(c.Text)
	}
	if joined.String() != sentence {
		return nil, report, fmt.Errorf("aligned chunks do not cover sentence exactly (got %d chars, want %d)", joined.Len(), len(sentence))
	}
	return out, report, nil
}

// locateChunk 在 sentence[fromIdx:] 中查找与 needle 匹配的一段
// 匹配规则：忽略空白差异 + 大小写不敏感；返回原句中的起止字节位置
func locateChunk(sentence string, fromIdx int, needle string) (int, int, bool) {
	needleCore := stripSpaces(needle)
	if needleCore == "" {
		return 0, 0, false
	}
	needleLower := strings.ToLower(needleCore)

	// 从 fromIdx 起，构造 sentence 的"无空白 lower 视图"到"原字节位置"的映射
	viewToOrig := make([]int, 0, len(sentence)-fromIdx+1)
	viewChars := make([]byte, 0, len(sentence)-fromIdx+1)
	for i := fromIdx; i < len(sentence); i++ {
		b := sentence[i]
		if isSpaceByte(b) {
			continue
		}
		viewChars = append(viewChars, toLowerByte(b))
		viewToOrig = append(viewToOrig, i)
	}

	view := string(viewChars)
	idx := strings.Index(view, needleLower)
	if idx < 0 {
		return 0, 0, false
	}
	startOrig := viewToOrig[idx]
	endView := idx + len(needleLower) - 1
	if endView >= len(viewToOrig) {
		return 0, 0, false
	}
	endOrig := viewToOrig[endView] + 1 // exclusive
	return startOrig, endOrig, true
}

// stripSpaces 移除所有空白（含制表符 / 换行）
func stripSpaces(s string) string {
	var b strings.Builder
	for _, r := range s {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

func toLowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// logAlignReport 把对齐过程的关键差异信息打进日志，便于快速判断"差一点还是差很多"
func logAlignReport(sentence string, in, out []model.SentenceChunk, r alignReport) {
	sentenceRunes := len([]rune(sentence))
	if r.Perfect {
		log.Printf("[sentence] align OK sentence=%q sentence_runes=%d chunk_count=%d (zero diff)",
			truncate(sentence, 200), sentenceRunes, len(out))
		return
	}
	// 差异不为零，打出详细报告
	log.Printf("[sentence] align adjusted sentence=%q sentence_runes=%d in_chunks=%d out_chunks=%d fills=%d trimmed_chars=%d adjusted_text=%d",
		truncate(sentence, 200), sentenceRunes, r.InChunks, r.OutChunks, r.InsertedFills, r.TrimmedChars, r.AdjustedText)

	// 大差异（>10 字符幻觉、或 >2 处补齐）额外打样本；否则只打 summary
	if r.TrimmedChars > 10 || r.InsertedFills > 2 || r.AdjustedText > 3 {
		for i, c := range in {
			log.Printf("[sentence]   in[%d] type=%s role=%q text=%q", i, c.Type, c.Role, truncate(c.Text, 120))
		}
		for i, c := range out {
			log.Printf("[sentence]   out[%d] type=%s role=%q text=%q", i, c.Type, c.Role, truncate(c.Text, 120))
		}
	}
}

// truncate 截断字符串用于日志，避免打印过长内容
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// salvageTruncatedSentenceResult 当模型因 max_tokens 截断导致返回的 JSON 不完整时，
// 尝试从残缺字符串中抢救出 structure 和已完整闭合的 chunk 对象，拼成合法 JSON。
func salvageTruncatedSentenceResult(content string) (llmSentenceResult, bool) {
	var result llmSentenceResult

	structure := salvageJSONStringField(content, "structure")
	result.Structure = structure

	arrStart := strings.Index(content, "\"chunks\"")
	if arrStart < 0 {
		if strings.TrimSpace(structure) != "" {
			return result, true
		}
		return result, false
	}
	bracket := strings.IndexByte(content[arrStart:], '[')
	if bracket < 0 {
		if strings.TrimSpace(structure) != "" {
			return result, true
		}
		return result, false
	}
	body := content[arrStart+bracket+1:]

	var items []string
	depth := 0
	inStr := false
	escape := false
	itemStart := -1
	for i := 0; i < len(body); i++ {
		c := body[i]
		if escape {
			escape = false
			continue
		}
		if inStr {
			if c == '\\' {
				escape = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if depth == 0 {
				itemStart = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && itemStart >= 0 {
				items = append(items, body[itemStart:i+1])
				itemStart = -1
			}
		case ']':
			if depth == 0 {
				goto done
			}
		}
	}
done:
	if len(items) == 0 && strings.TrimSpace(structure) == "" {
		return result, false
	}
	chunksJSON := "[" + strings.Join(items, ",") + "]"
	if err := json.Unmarshal([]byte(chunksJSON), &result.Chunks); err != nil {
		if strings.TrimSpace(structure) != "" {
			return result, true
		}
		return result, false
	}
	return result, true
}

// salvageJSONStringField 从可能截断的 JSON 中提取指定字符串字段的值。
// 只在字段值完整（有闭合引号）时返回；否则返回空字符串。
func salvageJSONStringField(content, field string) string {
	key := "\"" + field + "\""
	idx := strings.Index(content, key)
	if idx < 0 {
		return ""
	}
	rest := content[idx+len(key):]
	rest = strings.TrimLeft(rest, " \t\n\r:")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	var sb strings.Builder
	escape := false
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if escape {
			sb.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			sb.WriteByte(c)
			escape = true
			continue
		}
		if c == '"' {
			return sb.String()
		}
		sb.WriteByte(c)
	}
	return ""
}
