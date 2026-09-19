package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/db"
	"aaa_word/biz/dict"
	"aaa_word/biz/service"
)

type importRecord struct {
	Line int
	Text string
	Note string
}

type importFailure struct {
	Line     int
	ItemType string
	Text     string
	Stage    string
	Reason   string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: favoritebatch <import|clean> [flags]")
		return 2
	}
	root, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(stderr, "[favoritebatch] locate project root failed: %v\n", err)
		return 1
	}

	switch args[0] {
	case "import":
		return runImport(root, args[1:], stdout, stderr)
	case "clean":
		return runClean(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q; use import or clean\n", args[0])
		return 2
	}
}

func runImport(root string, args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("favoritebatch import", flag.ContinueOnError)
	flags.SetOutput(stderr)
	filePath := flags.String("file", "", "UTF-8 CSV file with text,note columns")
	sleep := flags.Duration("sleep", 500*time.Millisecond, "delay between external calls")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*filePath) == "" {
		fmt.Fprintln(stderr, "[favoritebatch] import requires -file")
		return 2
	}
	if *sleep < 0 {
		fmt.Fprintln(stderr, "[favoritebatch] sleep must not be negative")
		return 2
	}

	file, err := os.Open(*filePath)
	if err != nil {
		fmt.Fprintf(stderr, "[favoritebatch] open CSV failed: %v\n", err)
		return 1
	}
	records, failures, parseErr := readImportCSV(file)
	closeErr := file.Close()
	if parseErr != nil {
		failures = append(failures, importFailure{
			Line: 1, Stage: "validate", Reason: sanitizeFailureReason(parseErr),
		})
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "[favoritebatch] close CSV failed: %v\n", closeErr)
		return 1
	}
	if parseErr != nil {
		path, reportErr := writeFailureReport(root, failures, time.Now())
		if reportErr != nil {
			fmt.Fprintf(stderr, "[favoritebatch] write failure report failed: %v\n", reportErr)
			return 1
		}
		fmt.Fprintf(stderr, "[favoritebatch] invalid CSV; failure report=%s\n", path)
		return 1
	}
	parseFailureCount := len(failures)

	if err := dict.Init(config.ECDICTPath()); err != nil {
		fmt.Fprintf(stderr, "[favoritebatch] required ECDICT unavailable: %v\n", err)
		return 1
	}
	db.Init()
	if !db.Enabled() {
		fmt.Fprintln(stderr, "[favoritebatch] database not configured; set MYSQL_DSN")
		return 1
	}
	defer db.Close()

	stats := map[string]int{
		service.FavoriteBatchCreated:  0,
		service.FavoriteBatchExisting: 0,
		service.FavoriteBatchRepaired: 0,
	}
	for _, record := range records {
		itemType, _, _ := service.NormalizeFavoriteImportText(record.Text)
		result, err := service.ImportFavoriteItem(
			context.Background(),
			service.FavoriteImportItem{Text: record.Text, Note: record.Note},
			time.Now(),
			service.FavoriteBatchOptions{SleepBetweenCalls: *sleep},
		)
		if err != nil {
			failures = append(failures, importFailure{
				Line: record.Line, ItemType: itemType, Text: record.Text,
				Stage:  service.FavoriteBatchErrorStage(err),
				Reason: sanitizeFailureReason(err),
			})
			fmt.Fprintf(stderr, "[favoritebatch] FAIL line=%d type=%s text=%q stage=%s err=%v\n",
				record.Line, itemType, record.Text, service.FavoriteBatchErrorStage(err), err)
			continue
		}
		stats[result.Action]++
		fmt.Fprintf(stdout, "[favoritebatch] OK line=%d type=%s text=%q action=%s item_id=%d\n",
			record.Line, result.ItemType, result.Text, result.Action, result.ItemID)
	}

	fmt.Fprintf(stdout,
		"[favoritebatch] import done total=%d created=%d repaired=%d existing=%d failed=%d\n",
		len(records)+parseFailureCount, stats[service.FavoriteBatchCreated],
		stats[service.FavoriteBatchRepaired], stats[service.FavoriteBatchExisting], len(failures))
	if len(failures) == 0 {
		return 0
	}
	path, err := writeFailureReport(root, failures, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "[favoritebatch] write failure report failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stderr, "[favoritebatch] failure report=%s\n", path)
	return 1
}

func runClean(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("favoritebatch clean", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sleep := flags.Duration("sleep", 500*time.Millisecond, "delay between TTS calls")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sleep < 0 {
		fmt.Fprintln(stderr, "[favoritebatch] sleep must not be negative")
		return 2
	}

	db.Init()
	if !db.Enabled() {
		fmt.Fprintln(stderr, "[favoritebatch] database not configured; set MYSQL_DSN")
		return 1
	}
	defer db.Close()

	report, err := service.CleanFavoriteCollection(context.Background(), service.FavoriteBatchOptions{
		SleepBetweenCalls: *sleep,
	})
	if err != nil {
		fmt.Fprintf(stderr, "[favoritebatch] clean failed: %v\n", err)
		return 1
	}
	for _, failure := range report.Failures {
		fmt.Fprintf(stderr, "[favoritebatch] CLEAN FAIL type=%s item_id=%d text=%q stage=%s err=%v\n",
			failure.ItemType, failure.ItemID, failure.Text, failure.Stage, failure.Err)
	}
	fmt.Fprintf(stdout,
		"[favoritebatch] clean done scanned=%d healthy=%d audio_repaired=%d learning_inserted=%d failed=%d\n",
		report.Scanned, report.Healthy, report.AudioRepaired, report.LearningInserted, len(report.Failures))
	if len(report.Failures) > 0 {
		return 1
	}
	return 0
}

func readImportCSV(reader io.Reader) ([]importRecord, []importFailure, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	header, err := csvReader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("read CSV header: %w", err)
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\uFEFF")
	}
	index, err := parseImportHeader(header)
	if err != nil {
		return nil, nil, err
	}

	records := make([]importRecord, 0)
	failures := make([]importFailure, 0)
	for {
		row, readErr := csvReader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		line := csvErrorLine(readErr)
		if len(row) > 0 {
			line, _ = csvReader.FieldPos(0)
		}
		if readErr != nil {
			failures = append(failures, importFailure{
				Line: line, Stage: "validate", Reason: sanitizeFailureReason(readErr),
			})
			if _, ok := readErr.(*csv.ParseError); ok && len(row) == 0 {
				break
			}
			continue
		}
		if len(row) != len(header) {
			failures = append(failures, importFailure{
				Line: line, Stage: "validate",
				Reason: fmt.Sprintf("expected %d columns, got %d", len(header), len(row)),
			})
			continue
		}
		records = append(records, importRecord{
			Line: line,
			Text: row[index["text"]],
			Note: row[index["note"]],
		})
	}
	return records, failures, nil
}

func parseImportHeader(header []string) (map[string]int, error) {
	if len(header) != 2 {
		return nil, fmt.Errorf("CSV header must contain exactly text,note")
	}
	index := make(map[string]int, len(header))
	for i, raw := range header {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name != "text" && name != "note" {
			return nil, fmt.Errorf("unsupported CSV column %q", raw)
		}
		if _, exists := index[name]; exists {
			return nil, fmt.Errorf("duplicate CSV column %q", name)
		}
		index[name] = i
	}
	if _, ok := index["text"]; !ok {
		return nil, fmt.Errorf("CSV header missing text")
	}
	if _, ok := index["note"]; !ok {
		return nil, fmt.Errorf("CSV header missing note")
	}
	return index, nil
}

func writeFailureReport(root string, failures []importFailure, now time.Time) (string, error) {
	if len(failures) == 0 {
		return "", nil
	}
	temp, err := os.CreateTemp(root, ".favorite_import_failures_*.tmp")
	if err != nil {
		return "", fmt.Errorf("create failure report temp file: %w", err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	writer := csv.NewWriter(temp)
	if err := writer.Write([]string{"line", "item_type", "text", "stage", "reason"}); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("write failure report header: %w", err)
	}
	for _, failure := range failures {
		if err := writer.Write([]string{
			fmt.Sprintf("%d", failure.Line),
			failure.ItemType,
			failure.Text,
			failure.Stage,
			sanitizeFailureReason(errors.New(failure.Reason)),
		}); err != nil {
			_ = temp.Close()
			return "", fmt.Errorf("write failure report row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("flush failure report: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("sync failure report: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close failure report: %w", err)
	}

	finalPath := filepath.Join(root, "favorite_import_failures_"+now.Format("20060102_150405")+".csv")
	if err := os.Rename(tempPath, finalPath); err != nil {
		return "", fmt.Errorf("publish failure report: %w", err)
	}
	cleanup = false
	return finalPath, nil
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		content, readErr := os.ReadFile(filepath.Join(dir, "go.mod"))
		if readErr == nil && strings.Contains(string(content), "module aaa_word") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod with module aaa_word not found")
		}
		dir = parent
	}
}

func csvErrorLine(err error) int {
	var parseErr *csv.ParseError
	if errors.As(err, &parseErr) {
		return parseErr.Line
	}
	return 0
}

func sanitizeFailureReason(err error) string {
	if err == nil {
		return ""
	}
	parts := strings.Fields(err.Error())
	for i, part := range parts {
		trimmed := strings.TrimLeft(part, "\"'([{")
		if strings.HasPrefix(trimmed, "/") {
			parts[i] = "<redacted-path>"
		}
	}
	reason := strings.Join(parts, " ")
	const maxLength = 1000
	if len(reason) > maxLength {
		reason = reason[:maxLength] + "...(truncated)"
	}
	return reason
}
