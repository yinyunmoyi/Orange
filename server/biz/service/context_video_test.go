package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"github.com/DATA-DOG/go-sqlmock"
)

func validContextVideoRequest() *model.ContextVideoUploadRequest {
	return &model.ContextVideoUploadRequest{
		ContextID:         7,
		SourceFingerprint: strings.Repeat("a", 64),
		ClipStartMS:       10_000,
		ClipEndMS:         19_000,
		DurationMS:        9_000,
		ContentType:       "video/webm;codecs=vp8,opus",
		FileSize:          1024,
		Subtitles: []model.ContextVideoSubtitle{
			{StartMS: 0, EndMS: 4_000, Text: "This is the first line."},
			{StartMS: 4_000, EndMS: 9_000, Text: "This is the second line."},
		},
	}
}

func TestNormalizeContextVideoUpload_BitsUT(t *testing.T) {
	req := validContextVideoRequest()
	req.AudioContentType = "audio/webm;codecs=opus"
	req.AudioFileSize = 2048
	got, err := NormalizeContextVideoUpload(req)
	if err != nil {
		t.Fatalf("NormalizeContextVideoUpload() error = %v", err)
	}
	if got.SourceFingerprint != req.SourceFingerprint ||
		got.AudioContentType != req.AudioContentType ||
		got.AudioFileSize != req.AudioFileSize ||
		len(got.Subtitles) != 2 {
		t.Fatalf("normalized request = %+v", got)
	}
}

func TestNormalizeContextVideoUploadRejectsInvalidInput_BitsUT(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.ContextVideoUploadRequest)
	}{
		{name: "context", mutate: func(req *model.ContextVideoUploadRequest) { req.ContextID = 0 }},
		{name: "fingerprint", mutate: func(req *model.ContextVideoUploadRequest) { req.SourceFingerprint = "bad" }},
		{name: "mime", mutate: func(req *model.ContextVideoUploadRequest) { req.ContentType = "video/mp4" }},
		{name: "duration", mutate: func(req *model.ContextVideoUploadRequest) { req.DurationMS = 70_000 }},
		{name: "time mismatch", mutate: func(req *model.ContextVideoUploadRequest) { req.ClipEndMS = 30_000 }},
		{name: "size", mutate: func(req *model.ContextVideoUploadRequest) { req.FileSize = MaxContextVideoBytes + 1 }},
		{name: "audio mime", mutate: func(req *model.ContextVideoUploadRequest) {
			req.AudioContentType = "audio/mpeg"
			req.AudioFileSize = 1024
		}},
		{name: "audio size missing", mutate: func(req *model.ContextVideoUploadRequest) {
			req.AudioContentType = "audio/webm"
		}},
		{name: "audio too large", mutate: func(req *model.ContextVideoUploadRequest) {
			req.AudioContentType = "audio/webm"
			req.AudioFileSize = MaxContextAudioBytes + 1
		}},
		{name: "subtitles", mutate: func(req *model.ContextVideoUploadRequest) { req.Subtitles = nil }},
		{name: "subtitle range", mutate: func(req *model.ContextVideoUploadRequest) { req.Subtitles[0].EndMS = 99_000 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := validContextVideoRequest()
			test.mutate(req)
			if _, err := NormalizeContextVideoUpload(req); err != ErrContextVideoInvalid {
				t.Fatalf("error = %v, want %v", err, ErrContextVideoInvalid)
			}
		})
	}
}

func TestContextVideoDedupeKey_BitsUT(t *testing.T) {
	req := validContextVideoRequest()
	key := ContextVideoDedupeKey(req)
	if len(key) != 64 || key != ContextVideoDedupeKey(req) {
		t.Fatalf("unstable dedupe key %q", key)
	}
	changed := *req
	changed.ClipStartMS++
	if key == ContextVideoDedupeKey(&changed) {
		t.Fatal("different source range produced same key")
	}
}

func TestContextVideoVTT_BitsUT(t *testing.T) {
	row := &db.ContextVideo{
		SubtitlesJSON: `[{"startMs":0,"endMs":1250,"text":"First line"},{"startMs":1250,"endMs":9000,"text":"Second --> line"}]`,
	}
	got, err := ContextVideoVTT(row)
	if err != nil {
		t.Fatalf("ContextVideoVTT() error = %v", err)
	}
	for _, want := range []string{
		"WEBVTT\n\n",
		"00:00:00.000 --> 00:00:01.250",
		"00:00:01.250 --> 00:00:09.000",
		"Second --\u200b> line",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("VTT missing %q:\n%s", want, got)
		}
	}
}

func TestContextTaskDataIncludesSentenceRange_BitsUT(t *testing.T) {
	start, end := 12, 31
	task := &db.ContextTask{
		ID:                     3,
		Status:                 ContextTaskSucceeded,
		ExtractedSentenceStart: &start,
		ExtractedSentenceEnd:   &end,
	}
	data := contextTaskData(task)
	if data.SentenceStart == nil || *data.SentenceStart != start ||
		data.SentenceEnd == nil || *data.SentenceEnd != end {
		t.Fatalf("contextTaskData() = %+v", data)
	}
}

func TestSaveContextVideoReplacesTTSAudioAfterCommit_BitsUT(t *testing.T) {
	gormDB, mock, cleanup := newDeleteMockDB(t)
	defer cleanup()
	oldDB := db.DB
	db.DB = gormDB
	defer func() { db.DB = oldDB }()

	audioDir := t.TempDir()
	videoDir := t.TempDir()
	t.Setenv("AUDIO_DIR", audioDir)
	t.Setenv("CONTEXT_VIDEO_DIR", videoDir)
	oldAudioPath := filepath.Join(audioDir, "old-tts.mp3")
	if err := os.WriteFile(oldAudioPath, []byte("tts"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := validContextVideoRequest()
	req.AudioContentType = "audio/webm;codecs=opus"
	req.AudioFileSize = int64(len("original-audio"))

	mock.ExpectQuery("SELECT .* FROM `context_videos` WHERE dedupe_key = .*LIMIT").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `contexts` WHERE id = .*LIMIT .*FOR UPDATE").
		WithArgs(req.ContextID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "item_type", "item_id", "audio_file_path",
		}).AddRow(req.ContextID, ContextItemWord, int64(11), oldAudioPath))
	mock.ExpectQuery("SELECT `id` FROM `words` WHERE id = .*LIMIT .*FOR UPDATE").
		WithArgs(int64(11), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(11))
	mock.ExpectExec("UPDATE `contexts` SET .*audio_content_type.*audio_file_path.*audio_file_size.* WHERE id =").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `context_videos`").
		WillReturnResult(sqlmock.NewResult(19, 1))
	mock.ExpectCommit()

	data, err := SaveContextVideo(
		context.Background(),
		req,
		strings.NewReader("video-webm"),
		strings.NewReader("original-audio"),
	)
	if err != nil {
		t.Fatalf("SaveContextVideo() error = %v", err)
	}
	if data.ID != 19 {
		t.Fatalf("video id = %d, want 19", data.ID)
	}
	if _, statErr := os.Stat(oldAudioPath); !os.IsNotExist(statErr) {
		t.Fatalf("old TTS audio still exists, stat error = %v", statErr)
	}
	newAudioPath := filepath.Join(
		audioDir,
		"contexts",
		ContextVideoDedupeKey(req)[:2],
		ContextVideoDedupeKey(req)[2:4],
		ContextVideoDedupeKey(req)+".webm",
	)
	content, err := os.ReadFile(newAudioPath)
	if err != nil || string(content) != "original-audio" {
		t.Fatalf("original audio = %q, err = %v", content, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
