package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/converter"
	"aaa_word/biz/dict"
	"aaa_word/biz/model"
)

const (
	dictionaryMaxAttempts = 3
)

// ErrNotFound 未找到该单词的释义
var ErrNotFound = errors.New("no definitions found")

// ErrInvalidAudioSrc 音频源地址非法（SSRF 防护）
var ErrInvalidAudioSrc = errors.New("invalid audio source")

var errEmptyDictionaryResponse = errors.New("empty dictionary response")

var httpClient = &http.Client{Timeout: 15 * time.Second}
var dictionaryRetrySleep = time.Sleep

func dictionaryAPIBaseURL() string {
	return strings.TrimRight(config.DictionaryAPIBaseURL(), "/") + "/"
}

func isConfiguredDictionaryOrigin(candidate *url.URL) bool {
	base, err := url.Parse(config.DictionaryAPIBaseURL())
	return err == nil && candidate.Scheme == base.Scheme && candidate.Host == base.Host
}

type retryableDictionaryError struct {
	err error
}

func (e *retryableDictionaryError) Error() string {
	return e.err.Error()
}

func (e *retryableDictionaryError) Unwrap() error {
	return e.err
}

// Lookup 调用外部词典 API，返回组织好的单词数据
func Lookup(word string) (*model.WordData, error) {
	delays := [...]time.Duration{250 * time.Millisecond, 750 * time.Millisecond}
	var lastErr error
	for attempt := 1; attempt <= dictionaryMaxAttempts; attempt++ {
		log.Printf("[dict.lookup] attempt start word=%q attempt=%d/%d", word, attempt, dictionaryMaxAttempts)
		data, err := lookupDictionary(word, attempt)
		if err == nil {
			log.Printf("[dict.lookup] attempt success word=%q attempt=%d/%d",
				word, attempt, dictionaryMaxAttempts)
			return data, nil
		}
		lastErr = err
		var retryable *retryableDictionaryError
		if !errors.As(err, &retryable) || attempt == dictionaryMaxAttempts {
			log.Printf("[dict.lookup] give up word=%q attempt=%d/%d retryable=%t err=%v",
				word, attempt, dictionaryMaxAttempts, errors.As(err, &retryable), err)
			return nil, err
		}
		delay := delays[attempt-1]
		log.Printf("[dict.lookup] retry word=%q attempt=%d/%d delay=%s err=%v",
			word, attempt+1, dictionaryMaxAttempts, delay, err)
		dictionaryRetrySleep(delay)
	}
	return nil, lastErr
}

// LookupWithFallback 先查必装的本地 ECDICT。仅当 ECDICT 未收录词条时，
// 才回退 dictionaryapi.dev，最终以 TTS-only 数据兜底。
func LookupWithFallback(word string) (*model.WordData, error) {
	if data := lookupFromEcdict(word); data != nil {
		return data, nil
	}
	data, err := Lookup(word)
	if err != nil {
		return converter.BuildTTSOnlyWordData(word), err
	}
	return data, nil
}

// lookupFromEcdict 从 ECDICT 组装 WordData（含音标 + 英文释义 + 元数据 + TTS 发音）。
// 未收录该词时返回 nil。
func lookupFromEcdict(word string) *model.WordData {
	if !dict.Enabled() {
		return nil
	}
	entry, err := dict.Lookup(context.Background(), word)
	if err != nil {
		if !errors.Is(err, dict.ErrNotFound) {
			log.Printf("[dict.lookup] ecdict err word=%q err=%v", word, err)
		}
		return nil
	}
	meanings := dict.SplitDefinitions(entry.Definition)
	// 没有英文释义时也允许命中：至少音标 + 元数据可用；释义留空
	phonetic := dict.NormalizePhonetic(entry.Phonetic)
	data := &model.WordData{
		Word:     entry.Word,
		Phonetic: phonetic,
		Meanings: meanings,
		Pos:      dict.NormalizePosField(entry.Pos),
		Collins:  entry.Collins,
		Oxford:   entry.Oxford,
		Tags:     dict.SplitTags(entry.Tag),
		Exchange: entry.Exchange,
	}
	data.Pronunciations = converter.BuildUSPronunciations(entry.Word, phonetic)
	log.Printf("[dict.lookup] ecdict ok word=%q defs=%d meanings_groups=%d collins=%d",
		word, countDefs(data.Meanings), len(data.Meanings), entry.Collins)
	return data
}

func countDefs(ms []model.Meaning) int {
	n := 0
	for _, m := range ms {
		n += len(m.Definitions)
	}
	return n
}

func lookupDictionary(word string, attempt int) (*model.WordData, error) {
	reqURL := dictionaryAPIBaseURL() + url.PathEscape(word)
	if attempt > 1 {
		reqURL += "?_cache_bust=" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	t0 := time.Now()
	log.Printf("[dict.lookup] request start word=%q attempt=%d/%d url=%q",
		word, attempt, dictionaryMaxAttempts, reqURL)
	resp, err := httpClient.Get(reqURL)
	if err != nil {
		log.Printf("[dict.lookup] request failed word=%q attempt=%d/%d url=%q err=%v elapsed=%s",
			word, attempt, dictionaryMaxAttempts, reqURL, err, time.Since(t0))
		return nil, &retryableDictionaryError{err: fmt.Errorf("request dictionary api: %w", err)}
	}
	defer resp.Body.Close()

	elapsed := time.Since(t0)
	contentType := resp.Header.Get("Content-Type")
	contentLength := resp.Header.Get("Content-Length")
	log.Printf("[dict.lookup] response received word=%q attempt=%d/%d status=%d ct=%q cl=%q elapsed=%s",
		word, attempt, dictionaryMaxAttempts, resp.StatusCode, contentType, contentLength, elapsed)

	if resp.StatusCode == http.StatusNotFound {
		bodySnippet := readBodySnippet(resp.Body, 1024)
		log.Printf("[dict.lookup] not found word=%q url=%q status=404 bodyLen=%d body=%q",
			word, reqURL, len(bodySnippet), bodySnippet)
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		bodySnippet := readBodySnippet(resp.Body, 2048)
		log.Printf("[dict.lookup] unexpected status word=%q url=%q status=%d ct=%q bodyLen=%d body=%q elapsed=%s",
			word, reqURL, resp.StatusCode, contentType, len(bodySnippet), bodySnippet, elapsed)
		statusErr := fmt.Errorf("dictionary api status %d", resp.StatusCode)
		if transientDictionaryStatus(resp.StatusCode) {
			return nil, &retryableDictionaryError{err: statusErr}
		}
		return nil, statusErr
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[dict.lookup] read body failed word=%q err=%v elapsed=%s", word, err, time.Since(t0))
		return nil, &retryableDictionaryError{err: fmt.Errorf("read dictionary response: %w", err)}
	}
	if len(rawBody) == 0 {
		log.Printf("[dict.lookup] empty body word=%q elapsed=%s", word, time.Since(t0))
		return nil, &retryableDictionaryError{err: errEmptyDictionaryResponse}
	}
	var entries []model.DictEntry
	if err := json.Unmarshal(rawBody, &entries); err != nil {
		log.Printf("[dict.lookup] decode failed word=%q attempt=%d/%d err=%v bodyLen=%d body=%q",
			word, attempt, dictionaryMaxAttempts, err, len(rawBody), truncateBytes(rawBody, 500))
		return nil, &retryableDictionaryError{err: fmt.Errorf("decode dictionary response: %w", err)}
	}
	if len(entries) == 0 {
		log.Printf("[dict.lookup] empty entries word=%q bodyLen=%d body=%q",
			word, len(rawBody), truncateBytes(rawBody, 500))
		return nil, ErrNotFound
	}

	log.Printf("[dict.lookup] success word=%q attempt=%d/%d entries=%d bodyLen=%d elapsed=%s",
		word, attempt, dictionaryMaxAttempts, len(entries), len(rawBody), time.Since(t0))
	return converter.ToWordData(entries), nil
}

func transientDictionaryStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// readBodySnippet 从 body 中至多读取 max 字节用于日志
func readBodySnippet(r io.Reader, max int) string {
	b, err := io.ReadAll(io.LimitReader(r, int64(max)))
	if err != nil {
		return fmt.Sprintf("(read err: %v)", err)
	}
	return string(b)
}

// truncateBytes 用于日志裁剪 []byte，超长时加 "...(truncated)" 后缀
func truncateBytes(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

// StreamAudio 校验 src 后向其发起请求，返回响应体（由调用方流式拷贝并关闭）
func StreamAudio(src string) (io.ReadCloser, string, error) {
	if err := ValidateAudioSrc(src); err != nil {
		log.Printf("[audio.stream] validate reject src=%q err=%v", src, err)
		return nil, "", err
	}

	t0 := time.Now()
	resp, err := httpClient.Get(src)
	if err != nil {
		log.Printf("[audio.stream] upstream get err src=%q err=%v elapsed=%s", src, err, time.Since(t0))
		return nil, "", fmt.Errorf("download audio: %w", err)
	}
	log.Printf("[audio.stream] upstream got status src=%q status=%d ct=%q cl=%q elapsed=%s",
		src, resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Length"), time.Since(t0))
	if resp.StatusCode != http.StatusOK {
		bodySnippet := readBodySnippet(resp.Body, 1024)
		resp.Body.Close()
		log.Printf("[audio.stream] non-200 upstream src=%q status=%d bodyLen=%d body=%q",
			src, resp.StatusCode, len(bodySnippet), bodySnippet)
		return nil, "", fmt.Errorf("audio source status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	return resp.Body, contentType, nil
}

// ValidateAudioSrc 允许两类合法音频源：
// 1. 来自 api.dictionaryapi.dev 的 https 外链（原有词典音频）
// 2. 形如 tts://us/<word> 的内部特殊标识（走火山 TTS 兜底合成）
func ValidateAudioSrc(src string) error {
	u, err := url.Parse(src)
	if err != nil {
		return ErrInvalidAudioSrc
	}
	if isConfiguredDictionaryOrigin(u) {
		return nil
	}
	if u.Scheme == ttsScheme {
		if _, _, ok := ParseTTSSrc(src); ok {
			return nil
		}
	}
	return ErrInvalidAudioSrc
}
