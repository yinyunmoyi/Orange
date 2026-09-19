package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/db"
	"aaa_word/biz/dict"
	"aaa_word/biz/service"
)

// wordPattern 与 handler.wordPattern 一致：字母开头，允许字母/连字符/撇号
var wordPattern = regexp.MustCompile(`^[a-z][a-z'-]*$`)

// parsedGroup 一行解析后的结果
type parsedGroup struct {
	lineNo  int
	raw     string
	words   []string // 已规范化、去重
	skipped bool
	reason  string // skipped 或 failed 的原因
}

func main() {
	filePath := flag.String("file", "./data/similar_words.txt", "input file path (each line: word1,word2[,...])")
	failedFile := flag.String("failed-file", "", "failed lines output path (default: <input>.failed.txt)")
	forceRefresh := flag.Bool("force-refresh", false, "re-fetch even for words with complete phonetic, audio, and meanings")
	dryRun := flag.Bool("dry-run", false, "parse only, no DB writes and no network calls")
	flag.Parse()

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("[importsim] open file failed: %v", err)
	}
	defer f.Close()

	groups, totalWords := parseAll(f)

	log.Printf("[importsim] file=%s total_groups=%d total_words=%d force_refresh=%v dry_run=%v",
		*filePath, countValid(groups), totalWords, *forceRefresh, *dryRun)

	if *dryRun {
		printDryRun(groups)
		return
	}

	if err := dict.Init(config.ECDICTPath()); err != nil {
		log.Fatalf("[importsim] required ECDICT unavailable: %v", err)
	}
	db.Init()
	if !db.Enabled() {
		log.Fatalf("[importsim] db not enabled: please set MYSQL_DSN in .env or environment")
	}
	defer db.Close()

	failedLines := runImport(groups, *forceRefresh)
	outputPath := strings.TrimSpace(*failedFile)
	if outputPath == "" {
		outputPath = defaultFailedFilePath(*filePath)
	}
	if err := writeFailedGroups(outputPath, groups, failedLines); err != nil {
		log.Fatalf("[importsim] write failed lines file=%s err=%v", outputPath, err)
	}
	log.Printf("[importsim] failed lines written file=%s count=%d", outputPath, len(failedLines))
}

// parseAll 按行读取输入并解析出组
func parseAll(f *os.File) ([]parsedGroup, int) {
	var groups []parsedGroup
	total := 0
	scanner := bufio.NewScanner(f)
	// 允许较长的行
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		g := parsedGroup{lineNo: lineNo, raw: raw}
		parts := strings.Split(line, ",")
		seen := make(map[string]struct{}, len(parts))
		for _, p := range parts {
			w := strings.ToLower(strings.TrimSpace(p))
			if w == "" {
				continue
			}
			if !wordPattern.MatchString(w) {
				g.skipped = true
				g.reason = fmt.Sprintf("invalid word %q", w)
				break
			}
			if _, ok := seen[w]; ok {
				continue
			}
			seen[w] = struct{}{}
			g.words = append(g.words, w)
		}
		if !g.skipped && len(g.words) < 2 {
			g.skipped = true
			g.reason = fmt.Sprintf("less than 2 unique words: %v", g.words)
		}
		if !g.skipped {
			total += len(g.words)
		}
		groups = append(groups, g)
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("[importsim] read input failed: %v", err)
	}
	return groups, total
}

func countValid(groups []parsedGroup) int {
	n := 0
	for _, g := range groups {
		if !g.skipped {
			n++
		}
	}
	return n
}

func printDryRun(groups []parsedGroup) {
	for _, g := range groups {
		if g.skipped {
			log.Printf("[importsim] SKIP line=%d reason=%s raw=%q", g.lineNo, g.reason, g.raw)
			continue
		}
		log.Printf("[importsim] OK   line=%d words=%v", g.lineNo, g.words)
	}
}

// wordStats 汇总每种单词动作
type wordStats struct {
	fetched int
	cached  int
	missing int
	failed  int
}

// groupStats 汇总每种组动作
type groupStats struct {
	created int
	joined  int
	merged  int
	noop    int
	skipped int
	failed  int
}

func runImport(groups []parsedGroup, forceRefresh bool) []int {
	ctx := context.Background()

	// 单词缓存：一个词在多组间只处理一次
	type wordCache struct {
		id     int64
		action service.EnsureAction
	}
	cache := make(map[string]wordCache)

	var wordStat wordStats
	var groupStat groupStats
	failedLines := make([]int, 0)

	totalGroups := countValid(groups)
	gi := 0
	// 先统计要处理的单词总数（去重后跨组）
	wordsUniqueSet := make(map[string]struct{})
	for _, g := range groups {
		if g.skipped {
			continue
		}
		for _, w := range g.words {
			wordsUniqueSet[w] = struct{}{}
		}
	}
	totalWordsUnique := len(wordsUniqueSet)
	wi := 0

	for _, g := range groups {
		if g.skipped {
			groupStat.skipped++
			log.Printf("[importsim] group SKIP line=%d reason=%s raw=%q", g.lineNo, g.reason, g.raw)
			continue
		}
		gi++
		groupStart := time.Now()
		log.Printf("[importsim] group START (%d/%d) line=%d words=%v",
			gi, totalGroups, g.lineNo, g.words)

		// 1. 逐词联网初始化（每个词全局只做一次）
		wordIDs := make([]int64, 0, len(g.words))
		groupFailed := false
		for _, w := range g.words {
			if c, ok := cache[w]; ok {
				if c.id == 0 {
					// 之前解析失败（missing/failed），跳过这个词
					log.Printf("[importsim] word REUSE-FAILED word=%s action=%s line=%d",
						w, c.action, g.lineNo)
					failedLines = append(failedLines, g.lineNo)
					groupFailed = true
					continue
				}
				log.Printf("[importsim] word REUSE word=%s action=%s word_id=%d line=%d",
					w, c.action, c.id, g.lineNo)
				wordIDs = append(wordIDs, c.id)
				continue
			}
			wi++
			wordStart := time.Now()
			log.Printf("[importsim] word START (%d/%d) word=%s line=%d",
				wi, totalWordsUnique, w, g.lineNo)
			opts := service.EnsureOptions{ForceRefresh: forceRefresh}
			id, action, err := service.EnsureWordFullyFavorited(ctx, w, opts)
			elapsed := time.Since(wordStart)
			if err != nil {
				wordStat.failed++
				cache[w] = wordCache{id: 0}
				log.Printf("[importsim] word (%d/%d) FAIL word=%s reason=%v elapsed=%dms",
					wi, totalWordsUnique, w, err, elapsed.Milliseconds())
				failedLines = append(failedLines, g.lineNo)
				groupFailed = true
				continue
			}
			cache[w] = wordCache{id: id, action: action}
			switch action {
			case service.EnsureFetched:
				wordStat.fetched++
			case service.EnsureCached:
				wordStat.cached++
			case service.EnsureLookupMissing:
				wordStat.missing++
			}
			if id > 0 {
				wordIDs = append(wordIDs, id)
			} else {
				failedLines = append(failedLines, g.lineNo)
				groupFailed = true
			}
			log.Printf("[importsim] word (%d/%d) word=%s action=%s word_id=%d elapsed=%dms",
				wi, totalWordsUnique, w, action, id, elapsed.Milliseconds())
		}

		// 2. 组合并（要求至少 2 个有效 word_id）
		if len(wordIDs) < 2 {
			groupStat.failed++
			log.Printf("[importsim] group (%d/%d) FAIL line=%d reason=less_than_2_valid_words words=%v elapsed=%dms",
				gi, totalGroups, g.lineNo, g.words, time.Since(groupStart).Milliseconds())
			failedLines = append(failedLines, g.lineNo)
			continue
		}
		log.Printf("[importsim] group MERGE (%d/%d) line=%d valid_word_ids=%v",
			gi, totalGroups, g.lineNo, wordIDs)
		groupID, action, err := service.MergeGroup(ctx, wordIDs)
		elapsed := time.Since(groupStart)
		if err != nil {
			groupStat.failed++
			log.Printf("[importsim] group (%d/%d) FAIL line=%d words=%v reason=%v elapsed=%dms",
				gi, totalGroups, g.lineNo, g.words, err, elapsed.Milliseconds())
			failedLines = append(failedLines, g.lineNo)
			continue
		}
		switch action {
		case service.MergeCreated:
			groupStat.created++
		case service.MergeJoined:
			groupStat.joined++
		case service.MergeMerged:
			groupStat.merged++
		case service.MergeNoop:
			groupStat.noop++
		}
		note := ""
		if groupFailed {
			note = " (partial: some words failed)"
		}
		log.Printf("[importsim] group (%d/%d) line=%d words=%v action=%s group_id=%d elapsed=%dms%s",
			gi, totalGroups, g.lineNo, g.words, action, groupID, elapsed.Milliseconds(), note)
		if groupFailed {
			failedLines = append(failedLines, g.lineNo)
		}
	}

	failedLines = dedupInts(failedLines)
	log.Printf("[importsim] done words:{fetched=%d cached=%d missing=%d failed=%d} groups:{created=%d joined=%d merged=%d noop=%d skipped=%d failed=%d} failed_lines=%v",
		wordStat.fetched, wordStat.cached, wordStat.missing, wordStat.failed,
		groupStat.created, groupStat.joined, groupStat.merged, groupStat.noop, groupStat.skipped, groupStat.failed,
		failedLines)
	return failedLines
}

func dedupInts(v []int) []int {
	seen := make(map[int]struct{}, len(v))
	out := make([]int, 0, len(v))
	for _, x := range v {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

func defaultFailedFilePath(inputPath string) string {
	ext := filepath.Ext(inputPath)
	if ext == "" {
		return inputPath + ".failed"
	}
	return strings.TrimSuffix(inputPath, ext) + ".failed" + ext
}

func writeFailedGroups(outputPath string, groups []parsedGroup, failedLines []int) error {
	failed := make(map[int]struct{}, len(failedLines))
	for _, lineNo := range failedLines {
		failed[lineNo] = struct{}{}
	}
	var content strings.Builder
	for _, group := range groups {
		if _, ok := failed[group.lineNo]; !ok {
			continue
		}
		content.WriteString(group.raw)
		content.WriteByte('\n')
	}
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(content.String()), 0o644)
}
