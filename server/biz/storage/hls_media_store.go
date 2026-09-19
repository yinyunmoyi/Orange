package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aaa_word/biz/config"
)

// HLSMedia 是一份缓存后的 HLS 转封装产物。
type HLSMedia struct {
	Fingerprint string
	Directory   string
	PlaylistTS  time.Time
}

var (
	hlsFingerprintPattern = regexp.MustCompile(`^[a-f0-9]{16,128}$`)
	hlsSegmentPattern     = regexp.MustCompile(`^(seg[0-9]+\.(ts|m4s)|init\.mp4)$`)
	hlsPlaylistName       = "master.m3u8"
	hlsLockRegistry       sync.Map
	hlsLastAccessSeq      uint64
)

// HLSMediaDir 是所有 HLS 缓存产物的根目录。
func HLSMediaDir() string {
	return config.HLSMediaDir()
}

// HLSMediaCapacityBytes 是所有 HLS 缓存产物的空间上限，默认 20GB。
func HLSMediaCapacityBytes() int64 {
	return config.HLSMediaCapacityBytes()
}

// HLSMediaDirectory 返回指定 fingerprint 的缓存目录。
func HLSMediaDirectory(fingerprint string) (string, error) {
	if !hlsFingerprintPattern.MatchString(fingerprint) {
		return "", fmt.Errorf("invalid hls fingerprint")
	}
	return filepath.Join(HLSMediaDir(), fingerprint[:2], fingerprint), nil
}

// HLSPlaylistPath 返回 fingerprint 对应的 master.m3u8 路径。
func HLSPlaylistPath(fingerprint string) (string, error) {
	dir, err := HLSMediaDirectory(fingerprint)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, hlsPlaylistName), nil
}

// HLSSegmentPath 返回 fingerprint 下某个分片文件的路径，segmentName 必须匹配 seg\d+.ts。
func HLSSegmentPath(fingerprint, segmentName string) (string, error) {
	if !hlsSegmentPattern.MatchString(segmentName) {
		return "", fmt.Errorf("invalid hls segment name")
	}
	dir, err := HLSMediaDirectory(fingerprint)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, segmentName), nil
}

// HLSProfileVersion 描述当前 ffmpeg 参数版本。参数变更后 bump 此值，旧缓存自动失效。
const HLSProfileVersion = "v3-fmp4-aac"

// HLSMediaExists 判断缓存里 master.m3u8 是否已生成完成且 profile 与当前版本一致。
func HLSMediaExists(fingerprint string) (bool, error) {
	playlist, err := HLSPlaylistPath(fingerprint)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(playlist)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() || info.Size() <= 0 {
		return false, nil
	}
	profilePath, err := hlsProfilePath(fingerprint)
	if err != nil {
		return false, err
	}
	body, err := os.ReadFile(profilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(string(body)) == HLSProfileVersion, nil
}

// WriteHLSProfile 在临时目录里落一个 .profile sentinel，表示这批切片对应的 ffmpeg 参数版本。
func WriteHLSProfile(tmpDir string) error {
	return os.WriteFile(filepath.Join(tmpDir, ".profile"), []byte(HLSProfileVersion), 0o644)
}

func hlsProfilePath(fingerprint string) (string, error) {
	dir, err := HLSMediaDirectory(fingerprint)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".profile"), nil
}

// HLSMediaLock 为同一 fingerprint 的写入串行化，避免并发切片彼此覆盖。
func HLSMediaLock(fingerprint string) *sync.Mutex {
	actual, _ := hlsLockRegistry.LoadOrStore(fingerprint, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// TouchHLSMedia 更新目录访问时间，为 LRU 淘汰服务。
func TouchHLSMedia(fingerprint string) error {
	dir, err := HLSMediaDirectory(fingerprint)
	if err != nil {
		return err
	}
	seq := atomic.AddUint64(&hlsLastAccessSeq, 1)
	now := time.Now().Add(time.Duration(seq) * time.Nanosecond)
	return os.Chtimes(dir, now, now)
}

// PrepareHLSMediaDir 为新的切片任务准备一个空的临时目录。返回临时目录路径，调用方负责在完成后 CommitHLSMedia
// 或 AbortHLSMedia。
func PrepareHLSMediaDir(fingerprint string) (string, error) {
	dir, err := HLSMediaDirectory(fingerprint)
	if err != nil {
		return "", err
	}
	if mkErr := os.MkdirAll(filepath.Dir(dir), 0o755); mkErr != nil {
		return "", fmt.Errorf("mkdir parent: %w", mkErr)
	}
	tmp, tmpErr := os.MkdirTemp(filepath.Dir(dir), filepath.Base(dir)+".*.tmp")
	if tmpErr != nil {
		return "", fmt.Errorf("mkdir tmp: %w", tmpErr)
	}
	return tmp, nil
}

// CommitHLSMedia 将 PrepareHLSMediaDir 产生的临时目录原子替换到最终位置。
func CommitHLSMedia(fingerprint, tmpDir string) error {
	dir, err := HLSMediaDirectory(fingerprint)
	if err != nil {
		return err
	}
	_ = os.RemoveAll(dir)
	if err := os.Rename(tmpDir, dir); err != nil {
		return fmt.Errorf("commit hls dir: %w", err)
	}
	return nil
}

// AbortHLSMedia 清理未完成的临时目录。
func AbortHLSMedia(tmpDir string) {
	if tmpDir == "" {
		return
	}
	_ = os.RemoveAll(tmpDir)
}

// WriteHLSUpload 将上传的原始字节流写入临时目录，返回临时源文件路径。
func WriteHLSUpload(tmpDir string, body io.Reader) (string, int64, error) {
	sourcePath := filepath.Join(tmpDir, "source.bin")
	file, err := os.Create(sourcePath)
	if err != nil {
		return "", 0, fmt.Errorf("create source: %w", err)
	}
	size, copyErr := io.Copy(file, body)
	if closeErr := file.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(sourcePath)
		return "", 0, fmt.Errorf("write source: %w", copyErr)
	}
	return sourcePath, size, nil
}

type cacheEntry struct {
	fingerprint string
	path        string
	size        int64
	accessed    time.Time
}

// EnforceHLSMediaCapacity 按 LRU 策略淘汰缓存直到低于容量上限。
func EnforceHLSMediaCapacity() error {
	root := HLSMediaDir()
	capacity := HLSMediaCapacityBytes()
	if capacity <= 0 {
		return nil
	}
	entries, err := listHLSCache(root)
	if err != nil {
		return err
	}
	var total int64
	for _, entry := range entries {
		total += entry.size
	}
	if total <= capacity {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].accessed.Before(entries[j].accessed)
	})
	for _, entry := range entries {
		if total <= capacity {
			break
		}
		if err := os.RemoveAll(entry.path); err != nil {
			return fmt.Errorf("remove stale hls: %w", err)
		}
		total -= entry.size
	}
	return nil
}

func listHLSCache(root string) ([]cacheEntry, error) {
	shards, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read hls root: %w", err)
	}
	var entries []cacheEntry
	for _, shard := range shards {
		if !shard.IsDir() {
			continue
		}
		items, err := os.ReadDir(filepath.Join(root, shard.Name()))
		if err != nil {
			continue
		}
		for _, item := range items {
			if !item.IsDir() || !hlsFingerprintPattern.MatchString(item.Name()) {
				continue
			}
			dirPath := filepath.Join(root, shard.Name(), item.Name())
			info, err := os.Stat(dirPath)
			if err != nil {
				continue
			}
			size := dirSize(dirPath)
			entries = append(entries, cacheEntry{
				fingerprint: item.Name(),
				path:        dirPath,
				size:        size,
				accessed:    info.ModTime(),
			})
		}
	}
	return entries, nil
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
