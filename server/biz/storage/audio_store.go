package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"aaa_word/biz/config"
)

var audioVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// AudioDir 音频文件根目录；默认 ./data/audio，可通过 AUDIO_DIR 环境变量覆盖
func AudioDir() string {
	return config.AudioDir()
}

// PathFor 生成两级分片路径：{dir}/{word[0]}/{word[1]|_}/{word}_{accent}.mp3
// 单词长度不足时第二级用 "_"；扩展名统一 mp3（dictionaryapi.dev 就是 mp3）
func PathFor(word, accent string) string {
	if word == "" {
		word = "_"
	}
	first := string(word[0])
	second := "_"
	if len(word) > 1 {
		second = string(word[1])
	}
	fileName := fmt.Sprintf("%s_%s.mp3", word, accent)
	return filepath.Join(AudioDir(), first, second, fileName)
}

// SaveFromReader 原子写入：先写 *.tmp，成功后 rename 覆盖目标路径
// 失败会尝试清理临时文件；成功返回最终路径与写入字节数
func SaveFromReader(word, accent string, body io.Reader) (path string, size int64, err error) {
	path = PathFor(word, accent)
	return saveAt(path, body)
}

// PhraseAudioPath 使用短语哈希生成稳定路径，避免原文进入文件名。
func PhraseAudioPath(phrase string) string {
	sum := sha256.Sum256([]byte(phrase))
	key := hex.EncodeToString(sum[:])
	return filepath.Join(AudioDir(), "phrases", key[:2], key[2:4], key+".mp3")
}

// SavePhraseAudio 原子保存短语音频。
func SavePhraseAudio(phrase string, body io.Reader) (path string, size int64, err error) {
	return saveAt(PhraseAudioPath(phrase), body)
}

// FavoriteAudioVersionPath 为重新生成的收藏音频创建独立版本路径。
func FavoriteAudioVersionPath(itemType string, itemID int64, version string) (string, error) {
	if (itemType != "word" && itemType != "phrase") || itemID <= 0 || !audioVersionPattern.MatchString(version) {
		return "", fmt.Errorf("invalid favorite audio version path")
	}
	return filepath.Join(
		AudioDir(),
		"favorites",
		itemType,
		fmt.Sprintf("%d", itemID),
		version+".mp3",
	), nil
}

// SaveFavoriteAudioVersion 原子保存一份重新生成的收藏音频。
func SaveFavoriteAudioVersion(
	itemType string,
	itemID int64,
	version string,
	body io.Reader,
) (path string, size int64, err error) {
	path, err = FavoriteAudioVersionPath(itemType, itemID, version)
	if err != nil {
		return "", 0, err
	}
	return saveAt(path, body)
}

// ContextAudioPath 使用语境幂等键生成稳定的分片路径。
func ContextAudioPath(dedupeKey string) string {
	first, second := "__", "__"
	if len(dedupeKey) >= 2 {
		first = dedupeKey[:2]
	}
	if len(dedupeKey) >= 4 {
		second = dedupeKey[2:4]
	}
	return filepath.Join(AudioDir(), "contexts", first, second, dedupeKey+".mp3")
}

// SaveContextAudio 原子保存语境音频。
func SaveContextAudio(dedupeKey string, body io.Reader) (path string, size int64, err error) {
	return saveAt(ContextAudioPath(dedupeKey), body)
}

// ContextOriginalAudioPath 使用视频语境幂等键保存从源视频录制的 Opus 原声。
func ContextOriginalAudioPath(dedupeKey string) string {
	first, second := "00", "00"
	if len(dedupeKey) >= 4 {
		first, second = dedupeKey[:2], dedupeKey[2:4]
	}
	return filepath.Join(AudioDir(), "contexts", first, second, dedupeKey+".webm")
}

// SaveContextOriginalAudio 原子保存视频语境的原声音频。
func SaveContextOriginalAudio(dedupeKey string, body io.Reader) (path string, size int64, err error) {
	return saveAt(ContextOriginalAudioPath(dedupeKey), body)
}

func saveAt(path string, body io.Reader) (string, int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", 0, fmt.Errorf("mkdir: %w", err)
	}

	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", 0, fmt.Errorf("create tmp: %w", err)
	}

	size, err := io.Copy(f, body)
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("copy audio: %w", err)
	}

	if err = os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("rename: %w", err)
	}
	return path, size, nil
}

// Delete 删除单个音频文件；不存在视为成功，其他错误返回给调用方记录业务上下文。
func Delete(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete audio %s: %w", path, err)
	}
	return nil
}
