package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultFailedFilePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "./data/similar_words.txt", want: "./data/similar_words.failed.txt"},
		{input: "retry.csv", want: "retry.failed.csv"},
		{input: "retry", want: "retry.failed"},
	}
	for _, tt := range tests {
		if got := defaultFailedFilePath(tt.input); got != tt.want {
			t.Fatalf("defaultFailedFilePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWriteFailedGroupsPreservesInputOrderAndRawLines(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "retry", "failed.txt")
	groups := []parsedGroup{
		{lineNo: 2, raw: "alpha, beta"},
		{lineNo: 5, raw: "gamma,delta"},
		{lineNo: 8, raw: "epsilon,zeta"},
	}
	if err := writeFailedGroups(outputPath, groups, []int{8, 2, 8}); err != nil {
		t.Fatalf("writeFailedGroups() error = %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "alpha, beta\nepsilon,zeta\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestWriteFailedGroupsOverwritesWithEmptyFile(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "failed.txt")
	if err := os.WriteFile(outputPath, []byte("old failure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFailedGroups(outputPath, nil, nil); err != nil {
		t.Fatalf("writeFailedGroups() error = %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 0 {
		t.Fatalf("content = %q, want empty", content)
	}
}
