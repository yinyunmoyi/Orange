package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/service"
	"aaa_word/biz/storage"
)

// wordPattern 与 handler.wordPattern 保持一致
var wordPattern = regexp.MustCompile(`^[a-z][a-z'-]*$`)

func main() {
	wordsFlag := flag.String("words", "", "以逗号分隔的单词列表；例如 -words=creed,terraced")
	fileFlag := flag.String("file", "", "从文件读取要刷新的单词（每行一个，# 开头忽略）")
	accentFlag := flag.String("accent", "US", "只刷新指定 accent 的音频；空串表示刷新该词的所有 accent")
	sleepMS := flag.Int("sleep", 500, "每次 TTS 合成后休眠毫秒数，避免上游限流")
	dryRun := flag.Bool("dry-run", false, "只列出将要刷新的目标，不实际调用 TTS 和落盘")
	flag.Parse()

	words, err := collectWords(*wordsFlag, *fileFlag)
	if err != nil {
		log.Fatalf("[refreshaudio] collect words failed: %v", err)
	}
	if len(words) == 0 {
		log.Fatalf("[refreshaudio] no words specified; use -words=... or -file=...")
	}
	log.Printf("[refreshaudio] targets=%v accent_filter=%q sleep_ms=%d dry_run=%v",
		words, *accentFlag, *sleepMS, *dryRun)

	db.Init()
	if !db.Enabled() {
		log.Fatalf("[refreshaudio] db not enabled: please set MYSQL_DSN in .env or environment")
	}
	defer db.Close()

	stats := runStats{}
	sleep := time.Duration(*sleepMS) * time.Millisecond

	for i, w := range words {
		log.Printf("[refreshaudio] (%d/%d) start word=%q", i+1, len(words), w)
		if err := refreshOne(w, *accentFlag, *dryRun, sleep, &stats); err != nil {
			log.Printf("[refreshaudio] (%d/%d) FAIL word=%q err=%v", i+1, len(words), w, err)
			stats.failedWords = append(stats.failedWords, w)
			continue
		}
		log.Printf("[refreshaudio] (%d/%d) OK word=%q", i+1, len(words), w)
	}

	log.Printf("[refreshaudio] done total_words=%d refreshed=%d skipped=%d failed=%d failed_words=%v",
		len(words), stats.refreshed, stats.skipped, len(stats.failedWords), stats.failedWords)
	if len(stats.failedWords) > 0 {
		os.Exit(1)
	}
}

type runStats struct {
	refreshed   int
	skipped     int
	failedWords []string
}

// collectWords 合并 -words 与 -file 参数，返回去重且规范化的单词切片
func collectWords(wordsCSV, filePath string) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(raw string) error {
		w := strings.ToLower(strings.TrimSpace(raw))
		if w == "" {
			return nil
		}
		if !wordPattern.MatchString(w) {
			return fmt.Errorf("invalid word %q (must match %s)", raw, wordPattern.String())
		}
		if _, ok := seen[w]; ok {
			return nil
		}
		seen[w] = struct{}{}
		out = append(out, w)
		return nil
	}

	for _, part := range strings.Split(wordsCSV, ",") {
		if err := add(part); err != nil {
			return nil, err
		}
	}
	if filePath != "" {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", filePath, err)
		}
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if err := add(line); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// refreshOne 刷新单个词的音频：查 word_audios → TTS 合成 → 原子替换文件 → 更新 DB 元数据
func refreshOne(word, accentFilter string, dryRun bool, sleep time.Duration, stats *runStats) error {
	var wordRow db.Word
	if err := db.DB.Where("word = ?", word).Take(&wordRow).Error; err != nil {
		return fmt.Errorf("lookup word row: %w", err)
	}

	q := db.DB.Where("word_id = ?", wordRow.ID)
	if accentFilter != "" {
		q = q.Where("accent = ?", accentFilter)
	}
	var audios []db.WordAudio
	if err := q.Find(&audios).Error; err != nil {
		return fmt.Errorf("query audios: %w", err)
	}
	if len(audios) == 0 {
		stats.skipped++
		log.Printf("[refreshaudio] skip word=%q word_id=%d reason=no_matching_audio accent=%q",
			word, wordRow.ID, accentFilter)
		return nil
	}

	for _, a := range audios {
		if dryRun {
			log.Printf("[refreshaudio] DRY word=%q accent=%q file=%s source=%s",
				word, a.Accent, a.FilePath, a.SourceURL)
			continue
		}
		if err := refreshOneAudio(word, &a); err != nil {
			return fmt.Errorf("refresh accent=%q: %w", a.Accent, err)
		}
		stats.refreshed++
		if sleep > 0 {
			time.Sleep(sleep)
		}
	}
	return nil
}

// refreshOneAudio 针对一条 word_audios 记录，重新走 TTS 合成并覆盖本地文件
func refreshOneAudio(word string, audio *db.WordAudio) error {
	accent := strings.ToLower(strings.TrimSpace(audio.Accent))
	if accent == "" {
		accent = "us"
	}
	// 目前 TTS 仅支持美音；若已收藏为其它 accent，直接跳过并提示
	if accent != "us" {
		return fmt.Errorf("tts only supports us accent, got %q", audio.Accent)
	}

	t0 := time.Now()
	mp3Bytes, contentType, err := service.SynthesizeMP3(word, accent)
	if err != nil {
		return fmt.Errorf("synthesize tts: %w", err)
	}
	log.Printf("[refreshaudio] tts synth ok word=%q accent=%q bytes=%d ct=%s elapsed=%s",
		word, audio.Accent, len(mp3Bytes), contentType, time.Since(t0))

	// storage.SaveFromReader 内部走 *.tmp + rename 原子替换旧文件
	newPath, size, err := storage.SaveFromReader(word, audio.Accent, bytes.NewReader(mp3Bytes))
	if err != nil {
		return fmt.Errorf("save new audio: %w", err)
	}
	log.Printf("[refreshaudio] file replaced word=%q accent=%q old_path=%s new_path=%s new_size=%d old_size=%d",
		word, audio.Accent, audio.FilePath, newPath, size, audio.FileSize)

	// 新的 source_url 强制切到 tts://us/<word>，避免此后仍然指向 dictionaryapi CDN
	newSource := service.BuildTTSSrc(word, accent)
	updates := map[string]any{
		"file_path":    newPath,
		"file_size":    size,
		"content_type": contentType,
		"source_url":   newSource,
	}
	if err := db.DB.Model(&db.WordAudio{}).Where("id = ?", audio.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update word_audios: %w", err)
	}
	log.Printf("[refreshaudio] db updated word=%q accent=%q id=%d source_url=%s",
		word, audio.Accent, audio.ID, newSource)
	return nil
}
