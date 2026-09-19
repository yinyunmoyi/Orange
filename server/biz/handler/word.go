package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"
	"aaa_word/biz/service"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
)

// wordPattern 仅允许字母、连字符、撇号，防注入/SSRF
var wordPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z'-]*$`)

// phrasePattern 短语：2-8 个英文 token，token 间恰好 1 个 ASCII 空格，token 内规则同 wordPattern
var phrasePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z'-]*(?: [A-Za-z][A-Za-z'-]*){1,7}$`)

// searchPattern 收藏列表搜索关键字：兼容单词和短语，SQL 查询仍使用参数绑定。
var searchPattern = regexp.MustCompile(`^[a-zA-Z' -]{0,80}$`)

// normalizeWhitespace 把任意连续空白（空格、tab、换行、\u00A0 等）折叠为单个 ASCII 空格，并 trim 首尾。
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// logSnippet 对长字符串裁剪并保留 rune 边界，用于日志中打印完整/近似完整的原文
// 超过 max rune 数时追加 "...(truncated,total=N)" 后缀标记，避免日志刷屏但保留可查证据
func logSnippet(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + fmt.Sprintf("...(truncated,total=%d)", len(rs))
}

// posList 提取汉语释义的词性列表用于出参日志
func posList(meanings []model.ChineseMeaning) []string {
	out := make([]string, 0, len(meanings))
	for _, m := range meanings {
		out = append(out, m.PartOfSpeech)
	}
	return out
}

// Lookup 处理 POST /api/v1/word/lookup
func Lookup(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	var req model.LookupRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("[lookup] bind failed err=%v raw_body=%q remote=%s elapsed=%s",
			err, logSnippet(string(c.Request.Body()), 500), remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.BaseResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	log.Printf("[lookup] request raw_word=%q remote=%s", req.Word, remote)

	word := strings.TrimSpace(req.Word)
	if word == "" {
		log.Printf("[lookup] invalid word: empty raw=%q remote=%s elapsed=%s", req.Word, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.BaseResponse{Code: http.StatusBadRequest, Msg: "invalid word"})
		return
	}
	if !wordPattern.MatchString(word) {
		log.Printf("[lookup] invalid word: regex mismatch raw=%q trimmed=%q remote=%s elapsed=%s",
			req.Word, word, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.BaseResponse{Code: http.StatusBadRequest, Msg: "invalid word"})
		return
	}

	// 已收藏则直接从本地取
	if data, err := service.LoadWordDataFromDB(ctx, word); err == nil {
		defs := 0
		accents := make([]string, 0, len(data.Pronunciations))
		for _, m := range data.Meanings {
			defs += len(m.Definitions)
		}
		for _, p := range data.Pronunciations {
			accents = append(accents, p.Accent)
		}
		log.Printf("[lookup] hit local word=%q phonetic=%q accents=%v defs=%d elapsed=%s",
			word, data.Phonetic, accents, defs, time.Since(t0))
		c.JSON(http.StatusOK, model.BaseResponse{Code: 0, Data: data})
		return
	} else if !errors.Is(err, service.ErrNotFound) {
		log.Printf("[lookup] load-from-db failed word=%q err=%v remote=%s", word, err, remote)
		// 非"未收藏"错误：降级到外部 API，不阻塞用户
	}

	data, err := service.LookupWithFallback(word)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			// 词典 API 未收录（专有名词等）：仍返回一个只含 TTS 兜底发音的 WordData，
			// 让前端能播放合成音频；释义留空，由 /meaning 大模型接口补齐。
			log.Printf("[lookup] not found word=%q remote=%s elapsed=%s (returning tts-only fallback accents=%d)",
				word, remote, time.Since(t0), len(data.Pronunciations))
		} else {
			// service.Lookup 已对瞬时错误重试三次；重试耗尽后再降级，
			// 避免单次上游故障阻断收藏。
			log.Printf("[lookup] upstream retries exhausted word=%q err=%v remote=%s elapsed=%s (returning tts-only fallback accents=%d)",
				word, err, remote, time.Since(t0), len(data.Pronunciations))
		}
		c.JSON(http.StatusOK, model.BaseResponse{Code: 0, Data: data})
		return
	}

	defs := 0
	accents := make([]string, 0, len(data.Pronunciations))
	for _, m := range data.Meanings {
		defs += len(m.Definitions)
	}
	for _, p := range data.Pronunciations {
		accents = append(accents, p.Accent)
	}
	log.Printf("[lookup] ok word=%q phonetic=%q accents=%v defs=%d elapsed=%s",
		word, data.Phonetic, accents, defs, time.Since(t0))
	c.JSON(http.StatusOK, model.BaseResponse{Code: 0, Data: data})
}

// Audio 处理 GET /api/v1/word/audio?src=<url>
// 命中本地文件则直接读文件返回；未命中走外部代理流式转发。
func Audio(ctx context.Context, c *app.RequestContext) {
	start := time.Now()
	src := c.Query("src")
	log.Printf("[audio] request src=%q remote=%s", src, c.ClientIP())
	if src == "" {
		log.Printf("[audio] missing src, elapsed=%s", time.Since(start))
		c.JSON(http.StatusBadRequest, model.BaseResponse{Code: http.StatusBadRequest, Msg: "missing src"})
		return
	}
	if err := service.ValidateAudioSrc(src); err != nil {
		log.Printf("[audio] invalid src=%q err=%v elapsed=%s", src, err, time.Since(start))
		c.JSON(http.StatusBadRequest, model.BaseResponse{Code: http.StatusBadRequest, Msg: "invalid audio source"})
		return
	}

	// 本地命中：直接以文件形式返回
	if path, contentType, ok, err := service.LookupLocalAudioPath(ctx, src); ok {
		log.Printf("[audio] cache hit src=%q path=%s ct=%s elapsed=%s", src, path, contentType, time.Since(start))
		c.SetContentType(contentType)
		c.File(path)
		return
	} else if err != nil {
		log.Printf("[audio] cache lookup err src=%q err=%v", src, err)
	} else {
		log.Printf("[audio] cache miss src=%q -> proxy upstream", src)
	}

	// TTS 兜底：src 形如 tts://us/<word>，实时合成 MP3
	if word, accent, ok := service.ParseTTSSrc(src); ok {
		mp3Bytes, contentType, err := service.SynthesizeMP3(word, accent)
		if err != nil {
			if errors.Is(err, service.ErrTTSNotConfigured) {
				log.Printf("[audio] tts not configured src=%q word=%q accent=%q elapsed=%s",
					src, word, accent, time.Since(start))
				c.JSON(http.StatusServiceUnavailable, model.BaseResponse{Code: http.StatusServiceUnavailable, Msg: "tts api key not configured"})
				return
			}
			log.Printf("[audio] tts synth failed src=%q word=%q accent=%q err=%v elapsed=%s",
				src, word, accent, err, time.Since(start))
			c.JSON(http.StatusBadGateway, model.BaseResponse{Code: http.StatusBadGateway, Msg: "tts synthesize failed"})
			return
		}
		log.Printf("[audio] tts synth ok src=%q word=%q accent=%q bytes=%d ct=%s elapsed=%s",
			src, word, accent, len(mp3Bytes), contentType, time.Since(start))
		c.Data(http.StatusOK, contentType, mp3Bytes)
		return
	}

	body, contentType, err := service.StreamAudio(src)
	if err != nil {
		if errors.Is(err, service.ErrInvalidAudioSrc) {
			log.Printf("[audio] upstream reject src=%q err=%v elapsed=%s", src, err, time.Since(start))
			c.JSON(http.StatusBadRequest, model.BaseResponse{Code: http.StatusBadRequest, Msg: "invalid audio source"})
			return
		}
		// 词典 CDN 抽风（502/timeout）时，若能从 URL 反推 word+accent(US)，走 TTS 兜底
		if word, accent, ok := service.ParseDictAudioSrc(src); ok {
			log.Printf("[audio] upstream fail src=%q err=%v elapsed=%s -> tts fallback word=%q accent=%q",
				src, err, time.Since(start), word, accent)
			mp3Bytes, ct, tErr := service.SynthesizeMP3(word, accent)
			if tErr != nil {
				if errors.Is(tErr, service.ErrTTSNotConfigured) {
					log.Printf("[audio] tts fallback not configured src=%q word=%q accent=%q elapsed=%s",
						src, word, accent, time.Since(start))
					c.JSON(http.StatusServiceUnavailable, model.BaseResponse{Code: http.StatusServiceUnavailable, Msg: "tts api key not configured"})
					return
				}
				log.Printf("[audio] tts fallback failed src=%q word=%q accent=%q err=%v elapsed=%s",
					src, word, accent, tErr, time.Since(start))
				c.JSON(http.StatusBadGateway, model.BaseResponse{Code: http.StatusBadGateway, Msg: "fetch audio failed"})
				return
			}
			log.Printf("[audio] tts fallback ok src=%q word=%q accent=%q bytes=%d ct=%s elapsed=%s",
				src, word, accent, len(mp3Bytes), ct, time.Since(start))
			c.Data(http.StatusOK, ct, mp3Bytes)
			return
		}
		log.Printf("[audio] upstream fail src=%q err=%v elapsed=%s (no tts fallback available)",
			src, err, time.Since(start))
		c.JSON(http.StatusBadGateway, model.BaseResponse{Code: http.StatusBadGateway, Msg: "fetch audio failed"})
		return
	}

	log.Printf("[audio] upstream ok src=%q ct=%s elapsed=%s", src, contentType, time.Since(start))
	c.SetContentType(contentType)
	c.SetStatusCode(http.StatusOK)
	c.SetBodyStream(body, -1)
}

// Meaning 处理 POST /api/v1/word/meaning
func Meaning(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	word, ok := bindAndValidateWord(c, "meaning", func(code int, msg string) {
		c.JSON(code, model.MeaningResponse{Code: code, Msg: msg})
	})
	if !ok {
		return
	}
	log.Printf("[meaning] request word=%q remote=%s", word, remote)

	if data, err := service.LoadMeaningFromDB(ctx, word); err == nil {
		log.Printf("[meaning] hit local word=%q pos=%v elapsed=%s",
			word, posList(data.Meanings), time.Since(t0))
		c.JSON(http.StatusOK, model.MeaningResponse{Code: 0, Data: data})
		return
	} else if !errors.Is(err, service.ErrNotFound) {
		log.Printf("[meaning] load-from-db failed word=%q err=%v remote=%s", word, err, remote)
	}

	data, err := service.GetMeaning(word)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			log.Printf("[meaning] not found word=%q remote=%s elapsed=%s (returning empty)",
				word, remote, time.Since(t0))
			c.JSON(http.StatusOK, model.MeaningResponse{Code: 0, Data: &model.MeaningData{Word: word}})
			return
		}
		if errors.Is(err, service.ErrLLMNotConfigured) {
			log.Printf("[meaning] llm not configured word=%q remote=%s elapsed=%s",
				word, remote, time.Since(t0))
			c.JSON(http.StatusServiceUnavailable, model.MeaningResponse{Code: http.StatusServiceUnavailable, Msg: "llm api key not configured"})
			return
		}
		log.Printf("[meaning] failed word=%q err=%v remote=%s elapsed=%s",
			word, err, remote, time.Since(t0))
		c.JSON(http.StatusBadGateway, model.MeaningResponse{Code: http.StatusBadGateway, Msg: "get meaning failed"})
		return
	}

	log.Printf("[meaning] ok word=%q pos=%v elapsed=%s",
		word, posList(data.Meanings), time.Since(t0))
	c.JSON(http.StatusOK, model.MeaningResponse{Code: 0, Data: data})
}

// Explain 处理 POST /api/v1/word/explain：调用 LLM 返回一句自然汉语的语境解释
func Explain(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	var req model.ExplainRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("[explain] bind failed err=%v raw_body=%q remote=%s elapsed=%s",
			err, logSnippet(string(c.Request.Body()), 500), remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.ExplainResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	log.Printf("[explain] request raw_word=%q wordStart=%d wordEnd=%d ctx=%q remote=%s",
		req.Word, req.WordStart, req.WordEnd, logSnippet(req.Context, 300), remote)

	word := strings.TrimSpace(req.Word)
	word = normalizeWhitespace(word)
	isPhraseInput := strings.ContainsRune(word, ' ')
	if !isPhraseInput {
		if word == "" {
			log.Printf("[explain] invalid word: empty raw=%q remote=%s elapsed=%s",
				req.Word, remote, time.Since(t0))
			c.JSON(http.StatusBadRequest, model.ExplainResponse{Code: http.StatusBadRequest, Msg: "invalid word"})
			return
		}
		if !wordPattern.MatchString(word) {
			log.Printf("[explain] invalid word: regex mismatch raw=%q normalized=%q remote=%s elapsed=%s",
				req.Word, word, remote, time.Since(t0))
			c.JSON(http.StatusBadRequest, model.ExplainResponse{Code: http.StatusBadRequest, Msg: "invalid word"})
			return
		}
	} else {
		if !phrasePattern.MatchString(word) {
			log.Printf("[explain] invalid phrase raw=%q normalized=%q remote=%s elapsed=%s",
				req.Word, word, remote, time.Since(t0))
			c.JSON(http.StatusBadRequest, model.ExplainResponse{Code: http.StatusBadRequest, Msg: "invalid word"})
			return
		}
	}

	ctxText := normalizeSentence(req.Context)
	ctxText = normalizeWhitespace(ctxText)
	if ctxText == "" {
		log.Printf("[explain] empty context word=%q raw_ctx=%q remote=%s elapsed=%s",
			word, logSnippet(req.Context, 200), remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.ExplainResponse{Code: http.StatusBadRequest, Msg: "context is empty"})
		return
	}
	if len([]rune(ctxText)) > 1000 {
		log.Printf("[explain] context too long word=%q runes=%d normalized_ctx=%q remote=%s elapsed=%s",
			word, len([]rune(ctxText)), logSnippet(ctxText, 200), remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.ExplainResponse{Code: http.StatusBadRequest, Msg: "context too long"})
		return
	}

	start, end := req.WordStart, req.WordEnd
	valid := start >= 0 && end > start && end <= len(ctxText) &&
		strings.EqualFold(ctxText[start:end], word)
	if !valid {
		idx := strings.Index(strings.ToLower(ctxText), strings.ToLower(word))
		if idx < 0 {
			log.Printf("[explain] word not in context word=%q normalized_ctx=%q reqStart=%d reqEnd=%d ctxRunes=%d remote=%s elapsed=%s",
				word, logSnippet(ctxText, 300), req.WordStart, req.WordEnd, len([]rune(ctxText)), remote, time.Since(t0))
			c.JSON(http.StatusBadRequest, model.ExplainResponse{Code: http.StatusBadRequest, Msg: "word not found in context"})
			return
		}
		log.Printf("[explain] offset fallback word=%q reqStart=%d reqEnd=%d newStart=%d newEnd=%d",
			word, req.WordStart, req.WordEnd, idx, idx+len(word))
		start, end = idx, idx+len(word)
	}

	data, err := service.GetContextualExplanation(word, ctxText, start, end)
	if err != nil {
		if errors.Is(err, service.ErrLLMNotConfigured) {
			log.Printf("[explain] llm not configured word=%q remote=%s elapsed=%s",
				word, remote, time.Since(t0))
			c.JSON(http.StatusServiceUnavailable, model.ExplainResponse{Code: http.StatusServiceUnavailable, Msg: "llm api key not configured"})
			return
		}
		if errors.Is(err, service.ErrLLMBadOutput) {
			log.Printf("[explain] llm bad output word=%q err=%v remote=%s elapsed=%s",
				word, err, remote, time.Since(t0))
			c.JSON(http.StatusBadGateway, model.ExplainResponse{Code: http.StatusBadGateway, Msg: "explain failed"})
			return
		}
		log.Printf("[explain] failed word=%q err=%v remote=%s elapsed=%s",
			word, err, remote, time.Since(t0))
		c.JSON(http.StatusBadGateway, model.ExplainResponse{Code: http.StatusBadGateway, Msg: "explain failed"})
		return
	}

	log.Printf("[explain] ok word=%q explanation=%q elapsed=%s",
		word, logSnippet(data.Explanation, 300), time.Since(t0))
	c.JSON(http.StatusOK, model.ExplainResponse{Code: 0, Data: data})
}

// Phrase 处理 POST /api/v1/word/phrase：把整段短语作为整体交给 LLM 生成汉语释义
func Phrase(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	var req model.PhraseRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("[phrase] bind failed err=%v raw_body=%q remote=%s elapsed=%s",
			err, logSnippet(string(c.Request.Body()), 500), remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.MeaningResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	log.Printf("[phrase] request raw_phrase=%q remote=%s", req.Phrase, remote)

	phrase := normalizeWhitespace(req.Phrase)
	if phrase == "" {
		log.Printf("[phrase] empty raw=%q remote=%s elapsed=%s", req.Phrase, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.MeaningResponse{Code: http.StatusBadRequest, Msg: "invalid phrase"})
		return
	}
	if len(phrase) > 80 {
		log.Printf("[phrase] too long normalized=%q remote=%s elapsed=%s",
			phrase, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.MeaningResponse{Code: http.StatusBadRequest, Msg: "phrase too long"})
		return
	}
	if !phrasePattern.MatchString(phrase) {
		log.Printf("[phrase] invalid raw=%q normalized=%q remote=%s elapsed=%s",
			req.Phrase, phrase, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.MeaningResponse{Code: http.StatusBadRequest, Msg: "invalid phrase"})
		return
	}

	// 已收藏短语走 DB 直返，避免每次都请求 LLM 造成释义漂移
	if favored, phraseID, ferr := service.IsPhraseFavorited(ctx, phrase); ferr == nil && favored {
		if detail, derr := service.GetPhraseDetail(ctx, phraseID); derr == nil {
			data := &model.MeaningData{Word: phrase, Meanings: detail.Meanings}
			log.Printf("[phrase] hit local phrase=%q phrase_id=%d pos=%v elapsed=%s",
				phrase, phraseID, posList(data.Meanings), time.Since(t0))
			c.JSON(http.StatusOK, model.MeaningResponse{Code: 0, Data: data})
			return
		} else {
			log.Printf("[phrase] load-detail-from-db failed phrase=%q phrase_id=%d err=%v", phrase, phraseID, derr)
		}
	} else if ferr != nil {
		log.Printf("[phrase] check-favorite failed phrase=%q err=%v", phrase, ferr)
	}

	data, err := service.GetChinesePhraseMeaning(phrase)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			log.Printf("[phrase] not found phrase=%q remote=%s elapsed=%s (returning empty)",
				phrase, remote, time.Since(t0))
			c.JSON(http.StatusOK, model.MeaningResponse{Code: 0, Data: &model.MeaningData{Word: phrase}})
			return
		}
		if errors.Is(err, service.ErrLLMNotConfigured) {
			log.Printf("[phrase] llm not configured phrase=%q remote=%s elapsed=%s",
				phrase, remote, time.Since(t0))
			c.JSON(http.StatusServiceUnavailable, model.MeaningResponse{Code: http.StatusServiceUnavailable, Msg: "llm api key not configured"})
			return
		}
		log.Printf("[phrase] failed phrase=%q err=%v remote=%s elapsed=%s",
			phrase, err, remote, time.Since(t0))
		c.JSON(http.StatusBadGateway, model.MeaningResponse{Code: http.StatusBadGateway, Msg: "get phrase meaning failed"})
		return
	}

	log.Printf("[phrase] ok phrase=%q pos=%v elapsed=%s",
		phrase, posList(data.Meanings), time.Since(t0))
	c.JSON(http.StatusOK, model.MeaningResponse{Code: 0, Data: data})
}

// Contexts 处理 POST /api/v1/word/contexts
func Contexts(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	word, ok := bindAndValidateWord(c, "contexts", func(code int, msg string) {
		c.JSON(code, model.ContextsResponse{Code: code, Msg: msg})
	})
	if !ok {
		return
	}
	log.Printf("[contexts] request word=%q remote=%s", word, remote)

	if data, err := service.LoadContextsFromDB(ctx, word); err == nil {
		samples := make([]string, 0, len(data.Contexts))
		for _, cx := range data.Contexts {
			samples = append(samples, logSnippet(cx.Sentence, 80))
		}
		log.Printf("[contexts] hit local word=%q contexts=%v elapsed=%s", word, samples, time.Since(t0))
		c.JSON(http.StatusOK, model.ContextsResponse{Code: 0, Data: data})
		return
	} else if errors.Is(err, service.ErrNotFound) {
		log.Printf("[contexts] not found word=%q remote=%s elapsed=%s (returning empty)",
			word, remote, time.Since(t0))
		c.JSON(http.StatusOK, model.ContextsResponse{
			Code: 0,
			Data: &model.ContextsData{Word: word, Contexts: []model.Context{}},
		})
		return
	} else {
		log.Printf("[contexts] load-from-db failed word=%q err=%v remote=%s elapsed=%s",
			word, err, remote, time.Since(t0))
		c.JSON(http.StatusBadGateway, model.ContextsResponse{Code: http.StatusBadGateway, Msg: "get contexts failed"})
		return
	}
}

// Favorite 处理 POST /api/v1/word/favorite：将前端当前页面数据整体落库 + 下载音频
func Favorite(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	var req model.FavoriteRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("[favorite] bind failed err=%v raw_body=%q remote=%s elapsed=%s",
			err, logSnippet(string(c.Request.Body()), 500), remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.FavoriteResponse{Code: http.StatusBadRequest, Msg: "invalid request body"})
		return
	}
	word := strings.TrimSpace(req.Word)
	if word == "" {
		log.Printf("[favorite] invalid word: empty raw=%q remote=%s elapsed=%s",
			req.Word, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.FavoriteResponse{Code: http.StatusBadRequest, Msg: "invalid word"})
		return
	}
	if !wordPattern.MatchString(word) {
		log.Printf("[favorite] invalid word: regex mismatch raw=%q trimmed=%q remote=%s elapsed=%s",
			req.Word, word, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.FavoriteResponse{Code: http.StatusBadRequest, Msg: "invalid word"})
		return
	}
	req.Word = word
	// 出参汇总：可清楚看到前端上送了哪些数据段
	enGroups := 0
	if req.WordData != nil {
		enGroups = len(req.WordData.Meanings)
	}
	zhGroups := 0
	if req.Meaning != nil {
		zhGroups = len(req.Meaning.Meanings)
	}
	accents := []string{}
	if req.WordData != nil {
		for _, p := range req.WordData.Pronunciations {
			accents = append(accents, p.Accent)
		}
	}
	log.Printf("[favorite] request word=%q en_groups=%d zh_groups=%d accents=%v remote=%s",
		word, enGroups, zhGroups, accents, remote)

	itemID, level, err := service.AddFavorite(ctx, &req)
	if err != nil {
		if errors.Is(err, db.ErrDBDisabled) {
			log.Printf("[favorite] db disabled word=%q remote=%s elapsed=%s",
				word, remote, time.Since(t0))
			c.JSON(http.StatusServiceUnavailable, model.FavoriteResponse{Code: http.StatusServiceUnavailable, Msg: "database not configured"})
			return
		}
		log.Printf("[favorite] add failed word=%q err=%v remote=%s elapsed=%s",
			word, err, remote, time.Since(t0))
		c.JSON(http.StatusInternalServerError, model.FavoriteResponse{Code: http.StatusInternalServerError, Msg: err.Error()})
		return
	}

	log.Printf("[favorite] ok word=%q item_id=%d elapsed=%s", word, itemID, time.Since(t0))
	c.JSON(http.StatusOK, model.FavoriteResponse{
		Code: 0,
		Data: &model.FavoriteData{ItemID: itemID, Level: level},
	})
}

// FavoriteStatus 处理 GET /api/v1/word/favorite/status?word=xxx
func FavoriteStatus(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	raw := c.Query("word")
	word := strings.TrimSpace(raw)
	if word == "" {
		log.Printf("[favorite-status] invalid word: empty raw=%q remote=%s elapsed=%s",
			raw, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.FavoriteStatusResponse{Code: http.StatusBadRequest, Msg: "invalid word"})
		return
	}
	if !wordPattern.MatchString(word) {
		log.Printf("[favorite-status] invalid word: regex mismatch raw=%q remote=%s elapsed=%s",
			raw, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.FavoriteStatusResponse{Code: http.StatusBadRequest, Msg: "invalid word"})
		return
	}
	log.Printf("[favorite-status] request word=%q remote=%s", word, remote)

	ok, itemID, level, err := service.IsFavorited(ctx, word)
	if err != nil {
		if errors.Is(err, db.ErrDBDisabled) {
			log.Printf("[favorite-status] db disabled word=%q remote=%s elapsed=%s",
				word, remote, time.Since(t0))
			c.JSON(http.StatusOK, model.FavoriteStatusResponse{Code: 0, Data: &model.FavoriteStatusData{Favorited: false}})
			return
		}
		log.Printf("[favorite-status] query failed word=%q err=%v remote=%s elapsed=%s",
			word, err, remote, time.Since(t0))
		c.JSON(http.StatusInternalServerError, model.FavoriteStatusResponse{Code: http.StatusInternalServerError, Msg: err.Error()})
		return
	}
	log.Printf("[favorite-status] ok word=%q favorited=%t item_id=%d elapsed=%s", word, ok, itemID, time.Since(t0))
	c.JSON(http.StatusOK, model.FavoriteStatusResponse{Code: 0, Data: &model.FavoriteStatusData{Favorited: ok, ItemID: itemID, Level: level}})
}

// Favorites 处理 GET /api/v1/word/favorites?q=<optional>：返回已收藏词列表
func Favorites(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	rawQ := c.Query("q")
	query := strings.TrimSpace(rawQ)
	if !searchPattern.MatchString(query) {
		log.Printf("[favorites] invalid query raw=%q trimmed=%q remote=%s elapsed=%s",
			rawQ, query, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.FavoriteListResponse{Code: http.StatusBadRequest, Msg: "invalid query"})
		return
	}
	rawStart := strings.TrimSpace(c.Query("startAt"))
	rawEnd := strings.TrimSpace(c.Query("endAt"))
	rawLimit := strings.TrimSpace(c.Query("limit"))
	rawCursor := strings.TrimSpace(c.Query("cursor"))
	if rawStart != "" || rawEnd != "" || rawLimit != "" || rawCursor != "" {
		if query != "" || rawStart == "" || rawEnd == "" {
			c.JSON(http.StatusBadRequest, model.FavoritePageResponse{Code: http.StatusBadRequest, Msg: "invalid favorite page query"})
			return
		}
		start, startErr := time.Parse(time.RFC3339, rawStart)
		end, endErr := time.Parse(time.RFC3339, rawEnd)
		limit := 50
		var limitErr error
		if rawLimit != "" {
			limit, limitErr = strconv.Atoi(rawLimit)
		}
		if startErr != nil || endErr != nil || limitErr != nil || !start.Before(end) || limit < 1 || limit > 100 {
			log.Printf("[favorites-page] invalid start=%q end=%q limit=%q cursor_len=%d remote=%s",
				rawStart, rawEnd, rawLimit, len(rawCursor), remote)
			c.JSON(http.StatusBadRequest, model.FavoritePageResponse{Code: http.StatusBadRequest, Msg: "invalid favorite page query"})
			return
		}
		data, err := service.ListFavoritePage(ctx, start, end, limit, rawCursor)
		if err != nil {
			if errors.Is(err, service.ErrFavoritePageInvalid) {
				c.JSON(http.StatusBadRequest, model.FavoritePageResponse{Code: http.StatusBadRequest, Msg: "invalid favorite page cursor"})
				return
			}
			log.Printf("[favorites-page] list failed start=%s end=%s limit=%d err=%v remote=%s elapsed=%s",
				rawStart, rawEnd, limit, err, remote, time.Since(t0))
			c.JSON(http.StatusInternalServerError, model.FavoritePageResponse{Code: http.StatusInternalServerError, Msg: err.Error()})
			return
		}
		log.Printf("[favorites-page] ok count=%d has_more=%t remote=%s elapsed=%s",
			len(data.Items), data.NextCursor != "", remote, time.Since(t0))
		c.JSON(http.StatusOK, model.FavoritePageResponse{Code: 0, Data: data})
		return
	}
	log.Printf("[favorites] request q=%q remote=%s", query, remote)

	data, err := service.ListFavorites(ctx, query)
	if err != nil {
		if errors.Is(err, db.ErrDBDisabled) {
			log.Printf("[favorites] db disabled q=%q remote=%s elapsed=%s",
				query, remote, time.Since(t0))
			c.JSON(http.StatusOK, model.FavoriteListResponse{Code: 0, Data: &model.FavoriteListData{Items: []model.FavoriteListItem{}}})
			return
		}
		log.Printf("[favorites] list failed q=%q err=%v remote=%s elapsed=%s",
			query, err, remote, time.Since(t0))
		c.JSON(http.StatusInternalServerError, model.FavoriteListResponse{Code: http.StatusInternalServerError, Msg: err.Error()})
		return
	}
	items := make([]string, 0, len(data.Items))
	for _, it := range data.Items {
		items = append(items, it.ItemType+":"+it.Text)
	}
	log.Printf("[favorites] ok q=%q total=%d items=%v elapsed=%s",
		query, data.Total, items, time.Since(t0))
	c.JSON(http.StatusOK, model.FavoriteListResponse{Code: 0, Data: data})
}

func FavoriteGroups(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	data, err := service.ListFavoriteGroups(ctx, time.Now())
	if err != nil {
		if errors.Is(err, db.ErrDBDisabled) {
			c.JSON(http.StatusOK, model.FavoriteGroupListResponse{
				Code: 0, Data: &model.FavoriteGroupListData{Groups: []model.FavoriteGroupSummary{}},
			})
			return
		}
		log.Printf("[favorite-groups] failed remote=%s err=%v elapsed=%s", c.ClientIP(), err, time.Since(started))
		c.JSON(http.StatusInternalServerError, model.FavoriteGroupListResponse{
			Code: http.StatusInternalServerError, Msg: err.Error(),
		})
		return
	}
	log.Printf("[favorite-groups] ok groups=%d total=%d remote=%s elapsed=%s",
		len(data.Groups), data.Total, c.ClientIP(), time.Since(started))
	c.JSON(http.StatusOK, model.FavoriteGroupListResponse{Code: 0, Data: data})
}

// DeleteFavorite 处理 DELETE /api/v1/favorites/:itemType/:id。
func DeleteFavorite(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	itemType := strings.ToLower(strings.TrimSpace(c.Param("itemType")))
	itemID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || itemID <= 0 || (itemType != service.ContextItemWord && itemType != service.ContextItemPhrase) {
		log.Printf("[favorite-delete] invalid request remote=%s item_type=%q raw_id=%q err=%v elapsed=%s",
			c.ClientIP(), itemType, c.Param("id"), err, time.Since(started))
		c.JSON(http.StatusBadRequest, model.FavoriteDeleteResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid favorite delete request",
		})
		return
	}
	if err := service.DeleteFavorite(ctx, itemType, itemID); err != nil {
		switch {
		case errors.Is(err, service.ErrFavoriteDeleteInvalid):
			c.JSON(http.StatusBadRequest, model.FavoriteDeleteResponse{
				Code: http.StatusBadRequest,
				Msg:  "invalid favorite delete request",
			})
		case errors.Is(err, db.ErrDBDisabled):
			c.JSON(http.StatusServiceUnavailable, model.FavoriteDeleteResponse{
				Code: http.StatusServiceUnavailable,
				Msg:  "database not configured",
			})
		default:
			c.JSON(http.StatusInternalServerError, model.FavoriteDeleteResponse{
				Code: http.StatusInternalServerError,
				Msg:  "delete favorite failed",
			})
		}
		log.Printf("[favorite-delete] failed remote=%s item_type=%q item_id=%d err=%v elapsed=%s",
			c.ClientIP(), itemType, itemID, err, time.Since(started))
		return
	}
	log.Printf("[favorite-delete] ok remote=%s item_type=%q item_id=%d elapsed=%s",
		c.ClientIP(), itemType, itemID, time.Since(started))
	c.JSON(http.StatusOK, model.FavoriteDeleteResponse{Code: 0})
}

func normalizeWordAssociationRequest(
	req model.WordAssociationRequest,
) (model.WordAssociationRequest, bool) {
	req.Word = strings.ToLower(strings.TrimSpace(req.Word))
	req.MeaningHint = strings.TrimSpace(req.MeaningHint)
	req.SpellingHint = strings.TrimSpace(req.SpellingHint)
	if !wordPattern.MatchString(req.Word) {
		return model.WordAssociationRequest{}, false
	}
	if len([]rune(req.MeaningHint)) > 200 || len([]rune(req.SpellingHint)) > 200 {
		return model.WordAssociationRequest{}, false
	}
	return req, true
}

// WordAssociations 处理 POST /api/v1/word/associations。
func WordAssociations(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	remote := c.ClientIP()
	var raw model.WordAssociationRequest
	if err := c.BindJSON(&raw); err != nil {
		log.Printf("[word-association] bind failed err=%v remote=%s elapsed=%s",
			err, remote, time.Since(started))
		c.JSON(http.StatusBadRequest, model.WordAssociationResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid request body",
		})
		return
	}
	req, ok := normalizeWordAssociationRequest(raw)
	if !ok {
		log.Printf(
			"[word-association] invalid word=%q meaning_hint_runes=%d spelling_hint_runes=%d remote=%s elapsed=%s",
			raw.Word,
			len([]rune(strings.TrimSpace(raw.MeaningHint))),
			len([]rune(strings.TrimSpace(raw.SpellingHint))),
			remote,
			time.Since(started),
		)
		c.JSON(http.StatusBadRequest, model.WordAssociationResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid association request",
		})
		return
	}

	data, err := service.AssociateWords(
		ctx,
		req.Word,
		req.MeaningHint,
		req.SpellingHint,
	)
	if err != nil {
		if errors.Is(err, service.ErrLLMNotConfigured) {
			c.JSON(http.StatusServiceUnavailable, model.WordAssociationResponse{
				Code: http.StatusServiceUnavailable,
				Msg:  "llm api key not configured",
			})
			return
		}
		log.Printf("[word-association] failed word=%q err=%v remote=%s elapsed=%s",
			req.Word, err, remote, time.Since(started))
		c.JSON(http.StatusBadGateway, model.WordAssociationResponse{
			Code: http.StatusBadGateway,
			Msg:  "associate words failed",
		})
		return
	}
	log.Printf("[word-association] ok word=%q count=%d remote=%s elapsed=%s",
		req.Word, len(data.Items), remote, time.Since(started))
	c.JSON(http.StatusOK, model.WordAssociationResponse{Code: 0, Data: data})
}

func normalizeSimilarGroupMemberAddRequest(
	req model.SimilarGroupMemberAddRequest,
) (model.SimilarGroupMemberAddRequest, bool) {
	req.Word = strings.ToLower(strings.TrimSpace(req.Word))
	req.CandidateWord = strings.ToLower(strings.TrimSpace(req.CandidateWord))
	if req.Word == req.CandidateWord ||
		!wordPattern.MatchString(req.Word) ||
		!wordPattern.MatchString(req.CandidateWord) {
		return model.SimilarGroupMemberAddRequest{}, false
	}
	return req, true
}

// AddSimilarGroupMember handles POST /api/v1/word/groups/members.
func AddSimilarGroupMember(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	remote := c.ClientIP()
	var raw model.SimilarGroupMemberAddRequest
	if err := c.BindJSON(&raw); err != nil {
		log.Printf("[group-add] bind failed remote=%s err=%v elapsed=%s",
			remote, err, time.Since(started))
		c.JSON(http.StatusBadRequest, model.SimilarGroupDetailResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid request body",
		})
		return
	}
	req, ok := normalizeSimilarGroupMemberAddRequest(raw)
	if !ok {
		log.Printf("[group-add] invalid word=%q candidate=%q remote=%s elapsed=%s",
			raw.Word, raw.CandidateWord, remote, time.Since(started))
		c.JSON(http.StatusBadRequest, model.SimilarGroupDetailResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid similar group member request",
		})
		return
	}

	data, err := service.AddWordToSimilarGroup(ctx, req.Word, req.CandidateWord)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSimilarGroupInvalid):
			c.JSON(http.StatusBadRequest, model.SimilarGroupDetailResponse{
				Code: http.StatusBadRequest,
				Msg:  "invalid similar group member request",
			})
		case errors.Is(err, db.ErrDBDisabled), errors.Is(err, service.ErrLLMNotConfigured):
			c.JSON(http.StatusServiceUnavailable, model.SimilarGroupDetailResponse{
				Code: http.StatusServiceUnavailable,
				Msg:  "similar group service unavailable",
			})
		default:
			c.JSON(http.StatusBadGateway, model.SimilarGroupDetailResponse{
				Code: http.StatusBadGateway,
				Msg:  "add similar group member failed",
			})
		}
		log.Printf("[group-add] failed word=%q candidate=%q remote=%s err=%v elapsed=%s",
			req.Word, req.CandidateWord, remote, err, time.Since(started))
		return
	}
	log.Printf("[group-add] response ok word=%q candidate=%q group_id=%d members=%d remote=%s elapsed=%s",
		req.Word, req.CandidateWord, data.ID, len(data.Members), remote, time.Since(started))
	c.JSON(http.StatusOK, model.SimilarGroupDetailResponse{Code: 0, Data: data})
}

// SimilarGroups 处理 GET /api/v1/word/groups：返回全部相似单词组（不支持过滤参数）
func SimilarGroups(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	log.Printf("[groups] request remote=%s", remote)
	data, err := service.ListSimilarGroups(ctx)
	if err != nil {
		if errors.Is(err, db.ErrDBDisabled) {
			log.Printf("[groups] db disabled remote=%s elapsed=%s", remote, time.Since(t0))
			c.JSON(http.StatusOK, model.SimilarGroupListResponse{Code: 0, Data: &model.SimilarGroupListData{Items: []model.SimilarGroupSummary{}}})
			return
		}
		log.Printf("[groups] list failed err=%v remote=%s elapsed=%s", err, remote, time.Since(t0))
		c.JSON(http.StatusInternalServerError, model.SimilarGroupListResponse{Code: http.StatusInternalServerError, Msg: err.Error()})
		return
	}
	ids := make([]int64, 0, len(data.Items))
	for _, it := range data.Items {
		ids = append(ids, it.ID)
	}
	log.Printf("[groups] ok total=%d ids=%v elapsed=%s", data.Total, ids, time.Since(t0))
	c.JSON(http.StatusOK, model.SimilarGroupListResponse{Code: 0, Data: data})
}

func normalizeSimilarGroupWord(raw string) (string, bool) {
	word := strings.TrimSpace(raw)
	if word == "" || !wordPattern.MatchString(word) {
		return "", false
	}
	return strings.ToLower(word), true
}

// SimilarGroupByWord 处理 GET /api/v1/word/groups/by-word?word=xxx。
func SimilarGroupByWord(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	raw := c.Query("word")
	word, ok := normalizeSimilarGroupWord(raw)
	if !ok {
		log.Printf("[group-by-word] invalid word raw=%q remote=%s elapsed=%s",
			raw, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.SimilarGroupDetailResponse{
			Code: http.StatusBadRequest,
			Msg:  "invalid word",
		})
		return
	}
	log.Printf("[group-by-word] request word=%q remote=%s", word, remote)

	data, err := service.GetSimilarGroupByWord(ctx, word)
	if err != nil {
		if errors.Is(err, db.ErrDBDisabled) {
			log.Printf("[group-by-word] db disabled word=%q remote=%s elapsed=%s",
				word, remote, time.Since(t0))
			c.JSON(http.StatusOK, model.SimilarGroupDetailResponse{Code: 0})
			return
		}
		log.Printf("[group-by-word] failed word=%q err=%v remote=%s elapsed=%s",
			word, err, remote, time.Since(t0))
		c.JSON(http.StatusInternalServerError, model.SimilarGroupDetailResponse{
			Code: http.StatusInternalServerError,
			Msg:  err.Error(),
		})
		return
	}
	if data == nil {
		log.Printf("[group-by-word] not found word=%q remote=%s elapsed=%s",
			word, remote, time.Since(t0))
		c.JSON(http.StatusOK, model.SimilarGroupDetailResponse{Code: 0})
		return
	}
	log.Printf("[group-by-word] ok word=%q group_id=%d member_count=%d elapsed=%s",
		word, data.ID, data.MemberCount, time.Since(t0))
	c.JSON(http.StatusOK, model.SimilarGroupDetailResponse{Code: 0, Data: data})
}

// SimilarGroupDetail 处理 GET /api/v1/word/groups/:id：返回组内成员详情；未命中返回 200 + data=null
func SimilarGroupDetail(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		log.Printf("[group-detail] invalid id: parse failed raw=%q err=%v remote=%s elapsed=%s",
			idStr, err, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.SimilarGroupDetailResponse{Code: http.StatusBadRequest, Msg: "invalid group id"})
		return
	}
	if id <= 0 {
		log.Printf("[group-detail] invalid id: non-positive raw=%q parsed=%d remote=%s elapsed=%s",
			idStr, id, remote, time.Since(t0))
		c.JSON(http.StatusBadRequest, model.SimilarGroupDetailResponse{Code: http.StatusBadRequest, Msg: "invalid group id"})
		return
	}
	log.Printf("[group-detail] request id=%d remote=%s", id, remote)

	data, err := service.GetSimilarGroupDetail(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrDBDisabled) {
			log.Printf("[group-detail] db disabled id=%d remote=%s elapsed=%s",
				id, remote, time.Since(t0))
			c.JSON(http.StatusOK, model.SimilarGroupDetailResponse{Code: 0})
			return
		}
		log.Printf("[group-detail] failed id=%d err=%v remote=%s elapsed=%s",
			id, err, remote, time.Since(t0))
		c.JSON(http.StatusInternalServerError, model.SimilarGroupDetailResponse{Code: http.StatusInternalServerError, Msg: err.Error()})
		return
	}
	if data == nil {
		log.Printf("[group-detail] not found id=%d remote=%s elapsed=%s", id, remote, time.Since(t0))
		c.JSON(http.StatusOK, model.SimilarGroupDetailResponse{Code: 0})
		return
	}
	memberWords := make([]string, 0, len(data.Members))
	for _, m := range data.Members {
		memberWords = append(memberWords, m.Word)
	}
	log.Printf("[group-detail] ok id=%d member_count=%d members=%v elapsed=%s",
		id, data.MemberCount, memberWords, time.Since(t0))
	c.JSON(http.StatusOK, model.SimilarGroupDetailResponse{Code: 0, Data: data})
}

// sentencePattern 允许 ASCII 可打印字符（含空格）以及 Latin 脚本字母（含 café / naïve
// / résumé 等常见附加符号），另外放行 Unicode 组合标记以兼容 NFD 分解形式；长度另行控制。
// 仍会拒绝 CJK、emoji 等非 Latin 字符，避免注入非目标语言内容。
var sentencePattern = regexp.MustCompile(`^[\x20-\x7E\p{Latin}\p{M}]+$`)

// AnalyzeSentence 处理 POST /api/v1/word/sentence/analyze
// 采用 SSE 协议：立即返回 200 + text/event-stream header，等待期间每 10s 发一次心跳注释帧，
// 结果到达时用 event: result 帧承载 SentenceAnalysis JSON，最后 data: [DONE] 结束。
// 这样做的目的：DeepSeek 分析长句可能耗时较长，公网穿透网关可能因连接空闲而返回 504。
// 用 SSE 保活能让网关认为连接活跃，避开 Gateway Timeout。
func AnalyzeSentence(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	var req model.SentenceAnalyzeRequest
	bindErr := c.BindJSON(&req)

	c.Response.Header.Set("X-Accel-Buffering", "no")
	w := sse.NewWriter(c)
	defer w.Close()

	sendError := func(msg string) {
		payload, _ := json.Marshal(sseErrorPayload{Msg: msg})
		_ = w.Write(&sse.Event{Type: "error", Data: payload})
	}
	sendDone := func() {
		_ = w.Write(&sse.Event{Data: []byte("[DONE]")})
	}
	sendResult := func(data *model.SentenceAnalysis) {
		payload, err := json.Marshal(data)
		if err != nil {
			log.Printf("[sentence] marshal result failed err=%v sentence=%q remote=%s",
				err, logSnippet(req.Sentence, 200), remote)
			sendError("marshal result failed")
			return
		}
		if err := w.Write(&sse.Event{Type: "result", Data: payload}); err != nil {
			log.Printf("[sentence] write result failed err=%v sentence=%q remote=%s",
				err, logSnippet(req.Sentence, 200), remote)
		}
	}

	if bindErr != nil {
		log.Printf("[sentence] bind failed err=%v raw_body=%q remote=%s elapsed=%s",
			bindErr, logSnippet(string(c.Request.Body()), 500), remote, time.Since(t0))
		sendError("invalid request body")
		sendDone()
		return
	}
	log.Printf("[sentence] request raw_sentence=%q remote=%s", logSnippet(req.Sentence, 300), remote)

	sentence := normalizeSentence(req.Sentence)
	if sentence == "" {
		log.Printf("[sentence] empty sentence raw=%q remote=%s elapsed=%s",
			logSnippet(req.Sentence, 200), remote, time.Since(t0))
		sendError("sentence is empty")
		sendDone()
		return
	}
	if runes := len([]rune(sentence)); runes > 500 {
		log.Printf("[sentence] too long normalized=%q runes=%d remote=%s elapsed=%s",
			logSnippet(sentence, 300), runes, remote, time.Since(t0))
		sendError("sentence too long")
		sendDone()
		return
	}
	if !sentencePattern.MatchString(sentence) {
		log.Printf("[sentence] invalid chars sentence=%q remote=%s elapsed=%s",
			logSnippet(sentence, 300), remote, time.Since(t0))
		sendError("invalid sentence")
		sendDone()
		return
	}

	// 起 goroutine 跑 DeepSeek 分析，主循环轮询完成 chan 并按 10s 心跳
	type analyzeOutcome struct {
		data *model.SentenceAnalysis
		err  error
	}
	done := make(chan analyzeOutcome, 1)
	go func() {
		data, err := service.AnalyzeSentence(sentence)
		done <- analyzeOutcome{data: data, err: err}
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	heartbeatCount := 0
loop:
	for {
		select {
		case out := <-done:
			if out.err != nil {
				msg := "analyze failed"
				if errors.Is(out.err, service.ErrLLMNotConfigured) {
					msg = "llm api key not configured"
					log.Printf("[sentence] llm not configured sentence=%q remote=%s elapsed=%s",
						logSnippet(sentence, 200), remote, time.Since(t0))
				} else if errors.Is(out.err, service.ErrLLMBadOutput) {
					log.Printf("[sentence] llm bad output sentence=%q err=%v remote=%s elapsed=%s heartbeats=%d",
						logSnippet(sentence, 200), out.err, remote, time.Since(t0), heartbeatCount)
				} else {
					log.Printf("[sentence] analyze failed sentence=%q err=%v remote=%s elapsed=%s heartbeats=%d",
						logSnippet(sentence, 200), out.err, remote, time.Since(t0), heartbeatCount)
				}
				sendError(msg)
				sendDone()
				return
			}
			chunks := make([]string, 0, len(out.data.Chunks))
			for _, ch := range out.data.Chunks {
				chunks = append(chunks, logSnippet(ch.Text, 60))
			}
			log.Printf("[sentence] ok sentence=%q chunks=%v remote=%s elapsed=%s heartbeats=%d",
				logSnippet(sentence, 200), chunks, remote, time.Since(t0), heartbeatCount)
			sendResult(out.data)
			sendDone()
			break loop
		case <-ticker.C:
			heartbeatCount++
			if err := w.WriteKeepAlive(); err != nil {
				log.Printf("[sentence] keepalive failed sentence=%q err=%v elapsed=%s heartbeats=%d remote=%s",
					logSnippet(sentence, 200), err, time.Since(t0), heartbeatCount, remote)
				// 客户端断开，直接退出（后台 goroutine 也会随请求 ctx 取消，实际不会）
				return
			}
		}
	}
}

// sseErrorPayload 用于 event: error 的 data 载荷
type sseErrorPayload struct {
	Msg string `json:"msg"`
}

// sseDeltaPayload 用于 data: {delta:"..."}
type sseDeltaPayload struct {
	Delta string `json:"delta"`
}

// TranslateSentence 处理 POST /api/v1/word/sentence/translate（SSE 流式）
// 校验错误也走 SSE：先 hijack writer 发起 chunked 传输，再通过 event: error 帧告知客户端。
func TranslateSentence(ctx context.Context, c *app.RequestContext) {
	t0 := time.Now()
	remote := c.ClientIP()
	var req model.SentenceAnalyzeRequest
	bindErr := c.BindJSON(&req)

	c.Response.Header.Set("X-Accel-Buffering", "no")
	w := sse.NewWriter(c)
	defer w.Close()

	sendError := func(msg string) {
		payload, _ := json.Marshal(sseErrorPayload{Msg: msg})
		_ = w.Write(&sse.Event{Type: "error", Data: payload})
	}
	sendDone := func() {
		_ = w.Write(&sse.Event{Data: []byte("[DONE]")})
	}

	if bindErr != nil {
		log.Printf("[translate] bind failed err=%v raw_body=%q remote=%s elapsed=%s",
			bindErr, logSnippet(string(c.Request.Body()), 500), remote, time.Since(t0))
		sendError("invalid request body")
		sendDone()
		return
	}
	log.Printf("[translate] request raw_sentence=%q remote=%s", logSnippet(req.Sentence, 300), remote)

	sentence := normalizeSentence(req.Sentence)
	if sentence == "" {
		log.Printf("[translate] empty sentence raw=%q remote=%s elapsed=%s",
			logSnippet(req.Sentence, 200), remote, time.Since(t0))
		sendError("sentence is empty")
		sendDone()
		return
	}
	if runes := len([]rune(sentence)); runes > 500 {
		log.Printf("[translate] too long normalized=%q runes=%d remote=%s elapsed=%s",
			logSnippet(sentence, 300), runes, remote, time.Since(t0))
		sendError("sentence too long")
		sendDone()
		return
	}
	if !sentencePattern.MatchString(sentence) {
		log.Printf("[translate] invalid chars sentence=%q remote=%s elapsed=%s",
			logSnippet(sentence, 300), remote, time.Since(t0))
		sendError("invalid sentence")
		sendDone()
		return
	}

	var (
		deltaCount int
		accumBuf   strings.Builder
	)
	err := service.TranslateSentenceStream(sentence, func(delta string) {
		deltaCount++
		accumBuf.WriteString(delta)
		payload, _ := json.Marshal(sseDeltaPayload{Delta: delta})
		if err := w.Write(&sse.Event{Data: payload}); err != nil {
			log.Printf("[translate] write delta failed err=%v delta=%q sentence=%q",
				err, logSnippet(delta, 120), logSnippet(sentence, 200))
		}
	})
	if err != nil {
		msg := "translate failed"
		if errors.Is(err, service.ErrLLMNotConfigured) {
			msg = "llm api key not configured"
			log.Printf("[translate] llm not configured sentence=%q deltas=%d partial=%q remote=%s elapsed=%s",
				logSnippet(sentence, 200), deltaCount, logSnippet(accumBuf.String(), 300), remote, time.Since(t0))
		} else {
			log.Printf("[translate] stream error sentence=%q err=%v deltas=%d partial=%q remote=%s elapsed=%s",
				logSnippet(sentence, 200), err, deltaCount, logSnippet(accumBuf.String(), 300), remote, time.Since(t0))
		}
		sendError(msg)
		sendDone()
		return
	}
	log.Printf("[translate] ok sentence=%q deltas=%d translation=%q remote=%s elapsed=%s",
		logSnippet(sentence, 200), deltaCount, logSnippet(accumBuf.String(), 500), remote, time.Since(t0))
	sendDone()
}

// normalizeSentence 对用户输入做归一化：去首尾空白，把常见 unicode 引号/破折号替换为 ASCII 等价
func normalizeSentence(s string) string {
	s = strings.TrimSpace(s)
	replacer := strings.NewReplacer(
		"\u2018", "'", "\u2019", "'",
		"\u201C", "\"", "\u201D", "\"",
		"\u2013", "-", "\u2014", "-",
		"\u2026", "...",
		"\u00A0", " ",
	)
	return replacer.Replace(s)
}

// bindAndValidateWord 解析请求体并校验 word；校验失败时用 fail 回写错误响应并返回 ok=false
func bindAndValidateWord(c *app.RequestContext, tag string, fail func(code int, msg string)) (string, bool) {
	t0 := time.Now()
	remote := c.ClientIP()
	var req model.LookupRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("[%s] bind failed err=%v raw_body=%q remote=%s elapsed=%s",
			tag, err, logSnippet(string(c.Request.Body()), 500), remote, time.Since(t0))
		fail(http.StatusBadRequest, "invalid request body")
		return "", false
	}

	word := strings.TrimSpace(req.Word)
	if word == "" {
		log.Printf("[%s] invalid word: empty raw=%q remote=%s elapsed=%s",
			tag, req.Word, remote, time.Since(t0))
		fail(http.StatusBadRequest, "invalid word")
		return "", false
	}
	if !wordPattern.MatchString(word) {
		log.Printf("[%s] invalid word: regex mismatch raw=%q trimmed=%q remote=%s elapsed=%s",
			tag, req.Word, word, remote, time.Since(t0))
		fail(http.StatusBadRequest, "invalid word")
		return "", false
	}
	return word, true
}
