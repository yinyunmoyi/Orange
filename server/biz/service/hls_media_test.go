package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aaa_word/biz/storage"
)

// generateSampleMKV 用 ffmpeg 生成一段小的 mkv 文件用于集成测试。ffmpeg 缺失时跳过测试。
func generateSampleMKV(t *testing.T) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed on host, skip integration test")
	}
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "sample.mkv")
	cmd := exec.Command(
		"ffmpeg",
		"-loglevel", "error",
		"-y",
		"-f", "lavfi", "-i", "testsrc=size=160x120:rate=15:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "30",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "64k",
		"-shortest",
		dst,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("generate sample mkv: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read sample mkv: %v", err)
	}
	return data
}

func fingerprintFor(payload []byte) string {
	// 只用来构造有效 fingerprint，长度符合 [a-f0-9]{16,128}
	sum := hexDigest(payload)
	return sum
}

func hexDigest(payload []byte) string {
	// 简易 hash：截取十六进制表示的前 16 字符。测试内部使用。
	buf := make([]byte, 8)
	for i, b := range payload {
		buf[i%len(buf)] ^= b
	}
	return hex.EncodeToString(buf)
}

func withHLSTempDirs(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HLS_MEDIA_DIR", root)
}

func TestImportHLSMediaSuccess(t *testing.T) {
	withHLSTempDirs(t)
	payload := generateSampleMKV(t)
	fp := fingerprintFor(payload)

	result, err := ImportHLSMedia(context.Background(), fp, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("ImportHLSMedia failed: %v", err)
	}
	if result.CachedHit {
		t.Fatalf("expected first import to miss cache, got cachedHit=true")
	}
	if !strings.HasSuffix(result.PlaylistURL, "/master.m3u8") {
		t.Fatalf("unexpected playlist url: %s", result.PlaylistURL)
	}

	playlistPath, err := storage.HLSPlaylistPath(fp)
	if err != nil {
		t.Fatalf("HLSPlaylistPath failed: %v", err)
	}
	body, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("read playlist: %v", err)
	}
	if !strings.Contains(string(body), "#EXTM3U") {
		t.Fatalf("playlist missing EXTM3U header: %s", body)
	}
	if !strings.Contains(string(body), ".m4s") {
		t.Fatalf("playlist missing fMP4 segments: %s", body)
	}

	// 源文件应被删除以节省空间
	dir, _ := storage.HLSMediaDirectory(fp)
	if _, err := os.Stat(filepath.Join(dir, "source.bin")); !os.IsNotExist(err) {
		t.Fatalf("expected source.bin removed, err=%v", err)
	}
}

func TestImportHLSMediaCacheHit(t *testing.T) {
	withHLSTempDirs(t)
	payload := generateSampleMKV(t)
	fp := fingerprintFor(payload)

	if _, err := ImportHLSMedia(context.Background(), fp, bytes.NewReader(payload)); err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	result, err := ImportHLSMedia(context.Background(), fp, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("second import failed: %v", err)
	}
	if !result.CachedHit {
		t.Fatalf("expected cache hit on second import, got miss")
	}
}

func TestImportHLSMediaInvalidFingerprint(t *testing.T) {
	withHLSTempDirs(t)
	if _, err := ImportHLSMedia(context.Background(), "not-hex!!!!", bytes.NewReader([]byte{0x00})); err == nil {
		t.Fatalf("expected error for invalid fingerprint")
	}
}

func TestLookupHLSMediaStatus(t *testing.T) {
	withHLSTempDirs(t)
	payload := generateSampleMKV(t)
	fp := fingerprintFor(payload)

	before, err := LookupHLSMediaStatus(fp)
	if err != nil {
		t.Fatalf("status before import: %v", err)
	}
	if before.Ready {
		t.Fatalf("expected not ready before import")
	}
	if _, importErr := ImportHLSMedia(context.Background(), fp, bytes.NewReader(payload)); importErr != nil {
		t.Fatalf("import failed: %v", importErr)
	}
	after, err := LookupHLSMediaStatus(fp)
	if err != nil {
		t.Fatalf("status after import: %v", err)
	}
	if !after.Ready {
		t.Fatalf("expected ready after import")
	}
	if after.PlaylistURL == "" {
		t.Fatalf("expected playlist url after ready")
	}
}

func TestLookupHLSMediaStatusReturnsInFlightProgress(t *testing.T) {
	withHLSTempDirs(t)
	fp := "1234567890abcdef"
	setHLSProgress(fp, "transcoding", 47)
	t.Cleanup(func() {
		hlsProgressRegistry.Delete(fp)
	})

	status, err := LookupHLSMediaStatus(fp)
	if err != nil {
		t.Fatalf("lookup progress: %v", err)
	}
	if status.Ready || status.Phase != "transcoding" || status.Progress != 47 {
		t.Fatalf("unexpected progress status: %+v", status)
	}
}

func TestParseFFmpegTimestamp(t *testing.T) {
	seconds, err := parseFFmpegTimestamp("00:28:30.496000")
	if err != nil {
		t.Fatalf("parse timestamp: %v", err)
	}
	if math.Abs(seconds-1710.496) > 0.0001 {
		t.Fatalf("unexpected seconds: %f", seconds)
	}
	if _, err := parseFFmpegTimestamp("invalid"); err == nil {
		t.Fatal("expected invalid timestamp error")
	}
}
