package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadImportCSVSupportsBOMColumnOrderAndMultilineNote_BitsUT(t *testing.T) {
	input := "\uFEFFnote,text\n\"first line\nsecond line\",\"Look Up\"\nsimple,Hello\n"
	records, failures, err := readImportCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readImportCSV() error = %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("readImportCSV() failures = %+v", failures)
	}
	if len(records) != 2 {
		t.Fatalf("readImportCSV() records = %+v", records)
	}
	if records[0].Line != 2 || records[0].Text != "Look Up" ||
		records[0].Note != "first line\nsecond line" {
		t.Fatalf("first record = %+v", records[0])
	}
	if records[1].Line != 4 || records[1].Text != "Hello" || records[1].Note != "simple" {
		t.Fatalf("second record = %+v", records[1])
	}
}

func TestReadImportCSVRejectsUnexpectedHeader_BitsUT(t *testing.T) {
	for _, input := range []string{
		"text\nhello\n",
		"text,note,extra\nhello,note,value\n",
		"text,text\nhello,world\n",
	} {
		if _, _, err := readImportCSV(strings.NewReader(input)); err == nil {
			t.Fatalf("readImportCSV(%q) error = nil", input)
		}
	}
}

func TestReadImportCSVRecordsWrongColumnCountAsFailure_BitsUT(t *testing.T) {
	records, failures, err := readImportCSV(strings.NewReader("text,note\nhello\nworld,note\n"))
	if err != nil {
		t.Fatalf("readImportCSV() error = %v", err)
	}
	if len(records) != 1 || records[0].Text != "world" {
		t.Fatalf("records = %+v", records)
	}
	if len(failures) != 1 || failures[0].Line != 2 || failures[0].Stage != "validate" {
		t.Fatalf("failures = %+v", failures)
	}
}

func TestWriteFailureReport_BitsUT(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 9, 18, 45, 30, 0, time.Local)
	path, err := writeFailureReport(root, []importFailure{{
		Line: 7, ItemType: "phrase", Text: "look up", Stage: "audio",
		Reason: "save failed at /Users/example/private/audio.mp3",
	}}, now)
	if err != nil {
		t.Fatalf("writeFailureReport() error = %v", err)
	}
	wantPath := filepath.Join(root, "favorite_import_failures_20260809_184530.csv")
	if path != wantPath {
		t.Fatalf("writeFailureReport() path = %q, want %q", path, wantPath)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || strings.Join(rows[0], ",") != "line,item_type,text,stage,reason" {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[1][0] != "7" || rows[1][2] != "look up" || rows[1][4] != "save failed at <redacted-path>" {
		t.Fatalf("failure row = %+v", rows[1])
	}
}

func TestWriteFailureReportSkipsEmptyFailures_BitsUT(t *testing.T) {
	root := t.TempDir()
	path, err := writeFailureReport(root, nil, time.Now())
	if err != nil || path != "" {
		t.Fatalf("writeFailureReport(nil) = (%q, %v)", path, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unexpected files = %+v", entries)
	}
}

func TestFindProjectRoot_BitsUT(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "cmd", "tool")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module aaa_word\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)

	got, err := findProjectRoot()
	want, evalErr := filepath.EvalSymlinks(root)
	if evalErr != nil {
		t.Fatal(evalErr)
	}
	if err != nil || got != want {
		t.Fatalf("findProjectRoot() = (%q, %v), want (%q, nil)", got, err, want)
	}
}
