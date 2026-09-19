package config

import (
	"bufio"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 配置加载 —— 从项目根目录的 .env 读取键值对，并允许被真实环境变量覆盖。
// 不引入第三方依赖，自行解析 KEY=VALUE 行。

var (
	once   sync.Once
	values map[string]string
)

// load 只解析一次 .env，结果缓存到 values
func load() {
	values = make(map[string]string)

	path := findEnvFile()
	if path == "" {
		return
	}

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行与注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key != "" {
			values[key] = val
		}
	}
}

// findEnvFile 从当前工作目录逐级向上查找 .env（兼容不同启动目录）
func findEnvFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ".env")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // 到达根目录仍未找到
		}
		dir = parent
	}
}

// Get 返回配置项：真实环境变量优先，其次 .env，最后回退默认值
func Get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	once.Do(load)
	if v, ok := values[key]; ok && v != "" {
		return v
	}
	return def
}

func invalid(key, reason string) {
	log.Printf("[config] invalid %s (%s), using safe default", key, reason)
}

func URL(key, def string) string {
	raw := strings.TrimSpace(Get(key, def))
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		invalid(key, "expected absolute http(s) URL")
		return def
	}
	return strings.TrimRight(raw, "/")
}

func PositiveInt(key string, def int) int {
	value, err := strconv.Atoi(strings.TrimSpace(Get(key, strconv.Itoa(def))))
	if err != nil || value <= 0 {
		invalid(key, "expected positive integer")
		return def
	}
	return value
}

func PositiveInt64(key string, def int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(Get(key, strconv.FormatInt(def, 10))), 10, 64)
	if err != nil || value <= 0 {
		invalid(key, "expected positive integer")
		return def
	}
	return value
}

func Port(key string, def int) int {
	value := PositiveInt(key, def)
	if value > 65535 {
		invalid(key, "expected TCP port in range 1..65535")
		return def
	}
	return value
}

func DurationSeconds(key string, def time.Duration) time.Duration {
	seconds := PositiveInt64(key, int64(def/time.Second))
	return time.Duration(seconds) * time.Second
}

func CSV(key string, def []string) []string {
	raw := Get(key, strings.Join(def, ","))
	values := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(item); value != "" {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		invalid(key, "expected non-empty comma-separated list")
		return append([]string(nil), def...)
	}
	return values
}

func ServerHost() string               { return Get("SERVER_HOST", "0.0.0.0") }
func ServerPort() int                  { return Port("SERVER_PORT", 8888) }
func ServerMaxRequestBodyBytes() int64 { return PositiveInt64("SERVER_MAX_REQUEST_BODY_BYTES", 8<<30) }
func CORSAllowedOrigins() []string {
	return CSV("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://127.0.0.1:5173"})
}
func ServerLogPath() string        { return Get("SERVER_LOG_PATH", "./data/debug/server.log") }
func WebLogPath() string           { return Get("WEB_LOG_PATH", "./data/debug/web.log") }
func AndroidLogPath() string       { return Get("ANDROID_LOG_PATH", "./data/debug/android.ndjson") }
func ECDICTPath() string           { return Get("ECDICT_PATH", "./data/ecdict/stardict.db") }
func FFMPEGBinary() string         { return Get("FFMPEG_BINARY", "ffmpeg") }
func AudioDir() string             { return Get("AUDIO_DIR", "./data/audio") }
func ContextVideoDir() string      { return Get("CONTEXT_VIDEO_DIR", "./data/context-videos") }
func HLSMediaDir() string          { return Get("HLS_MEDIA_DIR", "./data/hls-media") }
func HLSMediaCapacityBytes() int64 { return PositiveInt64("HLS_MEDIA_CAPACITY_BYTES", 20<<30) }
func LLMAPIKey() string            { return Get("LLM_API_KEY", "") }
func LLMAPIURL() string            { return URL("LLM_API_URL", "https://api.deepseek.com/chat/completions") }
func LLMModel() string             { return Get("LLM_MODEL", "deepseek-v4-pro") }
func LLMTimeout() time.Duration    { return DurationSeconds("LLM_TIMEOUT_SECONDS", 180*time.Second) }
func DictionaryAPIBaseURL() string {
	return URL("DICTIONARY_API_BASE_URL", "https://api.dictionaryapi.dev/api/v2/entries/en")
}
func VolcTTSAPIURL() string {
	return URL("VOLC_TTS_API_URL", "https://openspeech.bytedance.com/api/v3/tts/unidirectional")
}
func VolcTTSAPIKey() string      { return Get("VOLC_TTS_API_KEY", "") }
func VolcTTSResourceID() string  { return Get("VOLC_TTS_RESOURCE_ID", "seed-tts-2.0") }
func VolcTTSSpeaker() string     { return Get("VOLC_TTS_SPEAKER", "") }
func TTSTimeout() time.Duration  { return DurationSeconds("TTS_TIMEOUT_SECONDS", 30*time.Second) }
func LearningDailyNewLimit() int { return PositiveInt("LEARNING_DAILY_NEW_LIMIT", 50) }
