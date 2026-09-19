package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"aaa_word/biz/config"
	"aaa_word/biz/storage"
)

// 与 HLS 转封装相关的错误。
var (
	ErrHLSMediaInvalid  = errors.New("invalid hls media")
	ErrHLSMediaNotFound = errors.New("hls media not found")
)

// MaxHLSMediaBytes 限制单个视频源文件的上限，默认 8GB。
const MaxHLSMediaBytes int64 = 8 << 30

// HLSMediaStatus 描述指定 fingerprint 是否已在缓存里。
type HLSMediaStatus struct {
	Fingerprint string `json:"fingerprint"`
	Ready       bool   `json:"ready"`
	PlaylistURL string `json:"playlistUrl,omitempty"`
	Phase       string `json:"phase,omitempty"`
	Progress    int    `json:"progress"`
}

// HLSImportResult 是 ImportHLSMedia 的响应。
type HLSImportResult struct {
	Fingerprint string `json:"fingerprint"`
	PlaylistURL string `json:"playlistUrl"`
	CachedHit   bool   `json:"cachedHit"`
	SourceSize  int64  `json:"sourceSize"`
}

type hlsProgressState struct {
	Phase    string
	Progress int
}

var hlsProgressRegistry sync.Map

// LookupHLSMediaStatus 查询指定 fingerprint 是否已就绪，无需触发切片。
func LookupHLSMediaStatus(fingerprint string) (*HLSMediaStatus, error) {
	if fingerprint == "" {
		return nil, ErrHLSMediaInvalid
	}
	exists, err := storage.HLSMediaExists(fingerprint)
	if err != nil {
		return nil, err
	}
	status := &HLSMediaStatus{Fingerprint: fingerprint, Ready: exists}
	if exists {
		status.PlaylistURL = playlistURL(fingerprint)
		status.Phase = "ready"
		status.Progress = 100
	} else if current, ok := hlsProgressRegistry.Load(fingerprint); ok {
		progress := current.(hlsProgressState)
		status.Phase = progress.Phase
		status.Progress = progress.Progress
	}
	return status, nil
}

// ImportHLSMedia 接收原始视频字节流，转封装到 HLS 缓存目录并返回可用的 playlist 地址。
// 如果 fingerprint 已经在缓存里，会直接返回缓存结果，body 会被完整消费掉以让上游连接干净关闭。
func ImportHLSMedia(ctx context.Context, fingerprint string, body io.Reader) (*HLSImportResult, error) {
	return ImportHLSMediaWithDuration(ctx, fingerprint, body, 0)
}

// ImportHLSMediaWithDuration 在转封装时使用源视频时长计算可查询的真实进度。
func ImportHLSMediaWithDuration(
	ctx context.Context,
	fingerprint string,
	body io.Reader,
	durationSeconds float64,
) (*HLSImportResult, error) {
	if fingerprint == "" || body == nil {
		return nil, ErrHLSMediaInvalid
	}
	if _, err := storage.HLSMediaDirectory(fingerprint); err != nil {
		return nil, ErrHLSMediaInvalid
	}

	mu := storage.HLSMediaLock(fingerprint)
	mu.Lock()
	defer mu.Unlock()

	if exists, err := storage.HLSMediaExists(fingerprint); err != nil {
		return nil, err
	} else if exists {
		_, _ = io.Copy(io.Discard, body)
		_ = storage.TouchHLSMedia(fingerprint)
		return &HLSImportResult{
			Fingerprint: fingerprint,
			PlaylistURL: playlistURL(fingerprint),
			CachedHit:   true,
		}, nil
	}
	setHLSProgress(fingerprint, "uploading", 0)

	tmpDir, err := storage.PrepareHLSMediaDir(fingerprint)
	if err != nil {
		return nil, fmt.Errorf("prepare hls dir: %w", err)
	}
	success := false
	defer func() {
		if !success {
			storage.AbortHLSMedia(tmpDir)
			setHLSProgress(fingerprint, "failed", 0)
		}
	}()

	sourcePath, size, err := storage.WriteHLSUpload(tmpDir, io.LimitReader(body, MaxHLSMediaBytes+1))
	if err != nil {
		return nil, err
	}
	if size <= 0 {
		return nil, ErrHLSMediaInvalid
	}
	if size > MaxHLSMediaBytes {
		return nil, ErrHLSMediaInvalid
	}

	setHLSProgress(fingerprint, "transcoding", 0)
	if err := transmuxToHLS(ctx, sourcePath, tmpDir, durationSeconds, func(progress int) {
		setHLSProgress(fingerprint, "transcoding", progress)
	}); err != nil {
		return nil, err
	}

	playlistTmp := filepath.Join(tmpDir, "master.m3u8")
	if info, statErr := os.Stat(playlistTmp); statErr != nil || info.Size() <= 0 {
		return nil, fmt.Errorf("hls playlist missing: %w", statErr)
	}

	if err := storage.WriteHLSProfile(tmpDir); err != nil {
		return nil, fmt.Errorf("write hls profile: %w", err)
	}

	// 源文件不再需要，删掉能节省一半磁盘。
	_ = os.Remove(sourcePath)

	if err := storage.CommitHLSMedia(fingerprint, tmpDir); err != nil {
		return nil, err
	}
	success = true
	setHLSProgress(fingerprint, "ready", 100)
	_ = storage.TouchHLSMedia(fingerprint)
	_ = storage.EnforceHLSMediaCapacity()

	return &HLSImportResult{
		Fingerprint: fingerprint,
		PlaylistURL: playlistURL(fingerprint),
		SourceSize:  size,
	}, nil
}

// ffmpegBinary 允许通过环境变量覆盖 ffmpeg 二进制路径，主要方便自动化测试。
func ffmpegBinary() string {
	return config.FFMPEGBinary()
}

// transmuxToHLS 把 sourcePath 转封装到 destDir，生成 master.m3u8 + seg%d.m4s。
// 使用 fMP4 分片而非 mpegts：hls.js 对 fMP4 兼容性更好，且不会踩 mpegts non-zero PTS
// 导致 audio SourceBuffer 建不起来的坑。视频用 -c:v copy 不重编码；音频强制转 AAC。
// 视频兼容失败时整体回落到 libx264。
func transmuxToHLS(
	ctx context.Context,
	sourcePath string,
	destDir string,
	durationSeconds float64,
	onProgress func(int),
) error {
	binary := ffmpegBinary()
	args := []string{
		"-loglevel", "error",
		"-y",
		"-i", sourcePath,
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "160k",
		"-ac", "2",
		"-avoid_negative_ts", "make_zero",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", filepath.Join(destDir, "seg%05d.m4s"),
		"-start_number", "0",
		filepath.Join(destDir, "master.m3u8"),
	}
	if err := runFFmpeg(ctx, binary, args, durationSeconds, onProgress); err == nil {
		return nil
	}

	// 首选路径失败时（例如源视频是 HEVC / AV1 等浏览器无法解码的视频编码），
	// 视频也一起重编码兜底一次，保证前端总能拿到可播的 HLS。
	fallbackArgs := []string{
		"-loglevel", "error",
		"-y",
		"-i", sourcePath,
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "22",
		"-c:a", "aac",
		"-b:a", "160k",
		"-ac", "2",
		"-avoid_negative_ts", "make_zero",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", filepath.Join(destDir, "seg%05d.m4s"),
		"-start_number", "0",
		filepath.Join(destDir, "master.m3u8"),
	}
	onProgress(0)
	return runFFmpeg(ctx, binary, fallbackArgs, durationSeconds, onProgress)
}

func runFFmpeg(
	ctx context.Context,
	binary string,
	args []string,
	durationSeconds float64,
	onProgress func(int),
) error {
	args = append([]string{"-progress", "pipe:1", "-nostats"}, args...)
	cmd := exec.CommandContext(ctx, binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	stderrDone := make(chan []byte, 1)
	go func() {
		stderrBytes, _ := io.ReadAll(stderr)
		stderrDone <- stderrBytes
	}()
	lastProgress := -1
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if durationSeconds <= 0 || !strings.HasPrefix(scanner.Text(), "out_time=") {
			continue
		}
		processedSeconds, parseErr := parseFFmpegTimestamp(strings.TrimPrefix(scanner.Text(), "out_time="))
		if parseErr != nil {
			continue
		}
		progress := int(processedSeconds / durationSeconds * 100)
		if progress > 99 {
			progress = 99
		}
		if progress >= 0 && progress != lastProgress {
			lastProgress = progress
			onProgress(progress)
		}
	}
	waitErr := cmd.Wait()
	stderrBytes := <-stderrDone
	if waitErr != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", waitErr, string(stderrBytes))
	}
	return nil
}

func parseFFmpegTimestamp(raw string) (float64, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid ffmpeg timestamp")
	}
	hours, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	minutes, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, err
	}
	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, err
	}
	return hours*3600 + minutes*60 + seconds, nil
}

func setHLSProgress(fingerprint, phase string, progress int) {
	if progress < 0 {
		progress = 0
	} else if progress > 100 {
		progress = 100
	}
	hlsProgressRegistry.Store(fingerprint, hlsProgressState{
		Phase:    phase,
		Progress: progress,
	})
}

func playlistURL(fingerprint string) string {
	return fmt.Sprintf("/api/v1/media/hls/%s/master.m3u8", fingerprint)
}
