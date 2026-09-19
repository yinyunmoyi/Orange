package service

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"aaa_word/biz/config"
)

const (
	ttsScheme     = "tts"
	ttsAccentUS   = "us"
	ttsAudioMIME  = "audio/mpeg"
	ttsSampleRate = 24000

	// ttsCodeSuccess 流中数据帧的成功码
	ttsCodeSuccess = 0
	// ttsCodeStreamEnd 流终止帧的成功码；火山用 20000000 + message="OK" 表示"整段合成完成"
	ttsCodeStreamEnd = 20000000

	// ttsSaneMaxBytes 单个单词/短语的合理音频大小上限（bytes）。
	// mp3@24kHz 单声道 CBR 32kbps ≈ 4KB/s；单词发音 <= 2s，正常 <=8KB。
	// 阈值取 30KB（~7.5s），超出视为大模型幻觉（把 "ridiculous" 读成整句话之类），需要重试。
	ttsSaneMaxBytes = 30 * 1024
	// ttsMaxRetries 幻觉重试次数（额外调用次数，不含首次）
	ttsMaxRetries = 1
)

// ErrTTSNotConfigured 火山 TTS 配置缺失（API Key 或 Speaker 未填）
var ErrTTSNotConfigured = errors.New("volc tts not configured")

// ErrTTSUpstream 火山 TTS 上游返回非 0 code 或响应不合法
var ErrTTSUpstream = errors.New("volc tts upstream error")

// ttsSrcWordPattern TTS 特殊标识 URL 中 word 部分的正则：允许字母、连字符、撇号、下划线
// 短语（含空格）在 tts:// 源里空格会被编码为 %20，url.Parse 已还原为 " "，一并允许
var ttsSrcWordPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z' \-]{0,79}$`)

var ttsClient = &http.Client{}

// ParseTTSSrc 解析 tts://<accent>/<word> 形式的音频源
// 命中且合法时返回 word, accent, true；否则返回 ok=false
func ParseTTSSrc(src string) (word, accent string, ok bool) {
	u, err := url.Parse(src)
	if err != nil {
		return "", "", false
	}
	if u.Scheme != ttsScheme {
		return "", "", false
	}
	accent = strings.ToLower(u.Host)
	// 当前仅支持美音
	if accent != ttsAccentUS {
		return "", "", false
	}
	word = strings.TrimPrefix(u.Path, "/")
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return "", "", false
	}
	if !ttsSrcWordPattern.MatchString(word) {
		return "", "", false
	}
	return word, accent, true
}

// dictAudioPattern dictionaryapi 音频路径：/media/pronunciations/en/<word>-<accent>.mp3
// 例：/media/pronunciations/en/interfere-us.mp3
var dictAudioPattern = regexp.MustCompile(`^/media/pronunciations/en/([a-zA-Z][a-zA-Z'\-]*)-([a-zA-Z]{2})\.mp3$`)

// ParseDictAudioSrc 解析 dictionaryapi.dev 的音频 URL，提取 word 与 accent
// 仅当 host、路径都匹配且 accent==us 时返回 ok=true
func ParseDictAudioSrc(src string) (word, accent string, ok bool) {
	u, err := url.Parse(src)
	if err != nil {
		return "", "", false
	}
	if !isConfiguredDictionaryOrigin(u) {
		return "", "", false
	}
	m := dictAudioPattern.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	word = strings.ToLower(m[1])
	accent = strings.ToLower(m[2])
	if accent != ttsAccentUS {
		return "", "", false
	}
	return word, accent, true
}

// BuildTTSSrc 构造 tts://<accent>/<word>
func BuildTTSSrc(word, accent string) string {
	w := strings.ToLower(strings.TrimSpace(word))
	a := strings.ToLower(strings.TrimSpace(accent))
	return ttsScheme + "://" + a + "/" + w
}

// ttsRequest 火山 v3 单向流式合成请求体
type ttsRequest struct {
	ReqParams ttsReqParams `json:"req_params"`
}

type ttsReqParams struct {
	Text        string         `json:"text"`
	Speaker     string         `json:"speaker"`
	AudioParams ttsAudioParams `json:"audio_params"`
	// Additions 火山接口要求是一个 JSON 字符串（内嵌 JSON），不是对象；
	// 所以这里用 string，调用方 marshal 成 `{"explicit_language":"en"}` 再赋值
	Additions string `json:"additions,omitempty"`
}

type ttsAudioParams struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
}

// ttsAdditions Additions 内嵌 JSON 的结构；先 Marshal 成 string 再放入 ttsReqParams.Additions
type ttsAdditions struct {
	ExplicitLanguage string `json:"explicit_language,omitempty"`
}

// ttsResponseFrame 火山响应流中的一帧 JSON
type ttsResponseFrame struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

// SynthesizeMP3 调用火山 TTS 合成单词/短语的 MP3；当前仅支持美音
// 返回完整 mp3 字节 + content-type。
// 为对抗大模型偶发的"补全整句"幻觉：
//  1. text 追加句号，给模型一个明确的句边界；
//  2. 首次结果超过 ttsSaneMaxBytes 视为可能幻觉，最多重试 ttsMaxRetries 次；
//  3. 全部超阈值则接受最后一次结果，避免死循环。
func SynthesizeMP3(word, accent string) ([]byte, string, error) {
	apiKey := config.VolcTTSAPIKey()
	resourceID := config.VolcTTSResourceID()
	speaker := config.VolcTTSSpeaker()
	if apiKey == "" || speaker == "" || resourceID == "" {
		log.Printf("[tts.synth] not configured word=%q accent=%q key_empty=%t speaker_empty=%t resource_empty=%t",
			word, accent, apiKey == "", speaker == "", resourceID == "")
		return nil, "", ErrTTSNotConfigured
	}
	if accent != ttsAccentUS {
		log.Printf("[tts.synth] unsupported accent word=%q accent=%q", word, accent)
		return nil, "", fmt.Errorf("%w: unsupported accent %q", ErrTTSUpstream, accent)
	}

	// 归一化 text：追加句号防止模型把单词补全成完整句子
	text := buildTTSText(word)

	var (
		lastBytes []byte
		lastCT    string
		lastErr   error
	)
	for attempt := 0; attempt <= ttsMaxRetries; attempt++ {
		mp3, ct, err := synthesizeOnce(word, accent, text, apiKey, resourceID, speaker, attempt)
		if err != nil {
			lastErr = err
			// 非致命错误也不重试；上层会返回失败让客户端稍后重试
			return nil, "", err
		}
		if len(mp3) <= ttsSaneMaxBytes {
			if attempt > 0 {
				log.Printf("[tts.synth] retry succeeded word=%q attempt=%d bytes=%d threshold=%d",
					word, attempt, len(mp3), ttsSaneMaxBytes)
			}
			return mp3, ct, nil
		}
		log.Printf("[tts.synth] suspicious size word=%q attempt=%d bytes=%d threshold=%d (likely hallucination, will retry if attempts left)",
			word, attempt, len(mp3), ttsSaneMaxBytes)
		lastBytes, lastCT = mp3, ct
	}
	// 重试次数用完，接受最后一次结果避免用户完全听不到
	log.Printf("[tts.synth] retries exhausted word=%q accept_bytes=%d threshold=%d last_err=%v",
		word, len(lastBytes), ttsSaneMaxBytes, lastErr)
	return lastBytes, lastCT, nil
}

// SynthesizeSentenceMP3 调用火山 TTS 合成完整英文句子。
// 句子音频通常大于单词音频，不使用单词场景的 30KB 异常阈值。
func SynthesizeSentenceMP3(sentence string) ([]byte, string, error) {
	text := strings.TrimSpace(sentence)
	if text == "" {
		log.Printf("[tts.sentence] empty sentence")
		return nil, "", fmt.Errorf("%w: empty sentence", ErrTTSUpstream)
	}
	if len([]rune(text)) > 500 {
		log.Printf("[tts.sentence] sentence too long runes=%d sentence=%q", len([]rune(text)), truncate(text, 200))
		return nil, "", fmt.Errorf("%w: sentence too long", ErrTTSUpstream)
	}

	apiKey := config.VolcTTSAPIKey()
	resourceID := config.VolcTTSResourceID()
	speaker := config.VolcTTSSpeaker()
	if apiKey == "" || speaker == "" || resourceID == "" {
		log.Printf("[tts.sentence] not configured key_empty=%t speaker_empty=%t resource_empty=%t",
			apiKey == "", speaker == "", resourceID == "")
		return nil, "", ErrTTSNotConfigured
	}

	return synthesizeOnce(text, ttsAccentUS, buildTTSText(text), apiKey, resourceID, speaker, 0)
}

// buildTTSText 归一化送给 TTS 的文本：trim + 若无终止标点则追加句号
func buildTTSText(word string) string {
	t := strings.TrimSpace(word)
	if t == "" {
		return t
	}
	last := t[len(t)-1]
	if last == '.' || last == '!' || last == '?' || last == ',' || last == ';' || last == ':' {
		return t
	}
	return t + "."
}

// synthesizeOnce 单次调用火山 TTS；不做归一化和重试
func synthesizeOnce(word, accent, text, apiKey, resourceID, speaker string, attempt int) ([]byte, string, error) {
	t0 := time.Now()
	apiURL := config.VolcTTSAPIURL()

	additionsBytes, err := json.Marshal(ttsAdditions{ExplicitLanguage: "en"})
	if err != nil {
		log.Printf("[tts.synth] marshal additions failed word=%q attempt=%d err=%v", word, attempt, err)
		return nil, "", fmt.Errorf("marshal tts additions: %w", err)
	}
	additionsStr := string(additionsBytes)

	body := ttsRequest{
		ReqParams: ttsReqParams{
			Text:    text,
			Speaker: speaker,
			AudioParams: ttsAudioParams{
				Format:     "mp3",
				SampleRate: ttsSampleRate,
			},
			Additions: additionsStr,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		log.Printf("[tts.synth] marshal request failed word=%q attempt=%d err=%v", word, attempt, err)
		return nil, "", fmt.Errorf("marshal tts request: %w", err)
	}

	reqID := newRequestID()
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("[tts.synth] build request failed word=%q attempt=%d err=%v", word, attempt, err)
		return nil, "", fmt.Errorf("build tts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("X-Api-Resource-Id", resourceID)
	req.Header.Set("X-Api-Request-Id", reqID)

	log.Printf("[tts.synth] request start word=%q accent=%q attempt=%d text=%q speaker=%q resource=%q req_id=%s url=%q additions=%s payload_len=%d payload=%s",
		word, accent, attempt, text, speaker, resourceID, reqID, apiURL, additionsStr, len(payload), string(payload))

	client := *ttsClient
	client.Timeout = config.TTSTimeout()
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[tts.synth] http do failed word=%q attempt=%d req_id=%s err=%v elapsed=%s",
			word, attempt, reqID, err, time.Since(t0))
		return nil, "", fmt.Errorf("do tts request: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[tts.synth] http response word=%q attempt=%d req_id=%s status=%d ct=%q cl=%q logid=%q elapsed=%s",
		word, attempt, reqID, resp.StatusCode,
		resp.Header.Get("Content-Type"), resp.Header.Get("Content-Length"),
		resp.Header.Get("X-Tt-Logid"), time.Since(t0))

	if resp.StatusCode != http.StatusOK {
		snippet := readBodySnippet(resp.Body, 2048)
		log.Printf("[tts.synth] non-200 word=%q attempt=%d status=%d req_id=%s body=%q elapsed=%s",
			word, attempt, resp.StatusCode, reqID, snippet, time.Since(t0))
		return nil, "", fmt.Errorf("%w: status %d", ErrTTSUpstream, resp.StatusCode)
	}

	// 响应是 chunked JSON 流；用 Decoder 逐帧解析并累积 base64 解码后的音频
	dec := json.NewDecoder(resp.Body)
	var buf bytes.Buffer
	frames := 0
	for {
		var frame ttsResponseFrame
		err := dec.Decode(&frame)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("[tts.synth] decode frame failed word=%q attempt=%d req_id=%s frames=%d err=%v elapsed=%s",
				word, attempt, reqID, frames, err, time.Since(t0))
			return nil, "", fmt.Errorf("decode tts frame: %w", err)
		}
		frames++
		if frame.Code == ttsCodeStreamEnd {
			log.Printf("[tts.synth] stream end word=%q attempt=%d req_id=%s frame_idx=%d code=%d message=%q total_bytes=%d elapsed=%s",
				word, attempt, reqID, frames, frame.Code, frame.Message, buf.Len(), time.Since(t0))
			break
		}
		if frame.Code != ttsCodeSuccess {
			log.Printf("[tts.synth] upstream error code word=%q attempt=%d req_id=%s code=%d message=%q frames=%d bytes_so_far=%d elapsed=%s",
				word, attempt, reqID, frame.Code, frame.Message, frames, buf.Len(), time.Since(t0))
			return nil, "", fmt.Errorf("%w: code=%d msg=%s", ErrTTSUpstream, frame.Code, frame.Message)
		}
		if frame.Data == "" {
			log.Printf("[tts.synth] frame empty data word=%q attempt=%d req_id=%s frame_idx=%d message=%q",
				word, attempt, reqID, frames, frame.Message)
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(frame.Data)
		if err != nil {
			log.Printf("[tts.synth] base64 decode failed word=%q attempt=%d req_id=%s frames=%d data_len=%d err=%v",
				word, attempt, reqID, frames, len(frame.Data), err)
			return nil, "", fmt.Errorf("base64 decode tts data: %w", err)
		}
		buf.Write(raw)
		log.Printf("[tts.synth] frame ok word=%q attempt=%d req_id=%s frame_idx=%d chunk_bytes=%d total_bytes=%d",
			word, attempt, reqID, frames, len(raw), buf.Len())
	}

	if buf.Len() == 0 {
		log.Printf("[tts.synth] empty audio word=%q attempt=%d req_id=%s frames=%d elapsed=%s",
			word, attempt, reqID, frames, time.Since(t0))
		return nil, "", fmt.Errorf("%w: empty audio", ErrTTSUpstream)
	}

	log.Printf("[tts.synth] ok word=%q accent=%q attempt=%d req_id=%s frames=%d bytes=%d elapsed=%s",
		word, accent, attempt, reqID, frames, buf.Len(), time.Since(t0))
	return buf.Bytes(), ttsAudioMIME, nil
}

// newRequestID 生成 32 位十六进制随机字符串作为 X-Api-Request-Id
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
