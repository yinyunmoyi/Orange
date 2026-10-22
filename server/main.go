package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/db"
	"aaa_word/biz/dict"
	"aaa_word/biz/discovery"
	"aaa_word/biz/handler"
	"aaa_word/biz/reverselan"
	"aaa_word/biz/router"
	"aaa_word/biz/syncidentity"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func main() {
	// -port 保留为显式最高优先级；未传入时读取统一配置。
	portFlag := flag.Int("port", 0, "HTTP server listen port (overrides SERVER_PORT)")
	flag.Parse()
	port := config.ServerPort()
	if *portFlag != 0 {
		if *portFlag < 1 || *portFlag > 65535 {
			log.Fatalf("invalid -port: expected 1..65535")
		}
		port = *portFlag
	}

	// #region debug-point A-B-E:process-lifecycle
	reportDebugEvent("A,B,E", "main.go:startup", "[DEBUG] server process starting", map[string]any{
		"pid": os.Getpid(), "host": config.ServerHost(), "port": port, "goroutines": runtime.NumGoroutine(),
	})
	signal.Ignore(syscall.SIGHUP)
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalCh)
	go func() {
		sig := <-signalCh
		reportDebugEvent("E", "main.go:signal", "[DEBUG] server received exit signal", map[string]any{
			"pid": os.Getpid(), "signal": sig.String(), "goroutines": runtime.NumGoroutine(),
		})
	}()
	// #endregion

	closeServerLog := enableServerLog()
	defer closeServerLog()

	db.Init()
	defer db.Close()

	if err := dict.Init(config.ECDICTPath()); err != nil {
		log.Fatalf("[ecdict] required dictionary unavailable: %v", err)
	}

	if serverID, err := syncidentity.Current(); err != nil {
		log.Printf("[context-video-sync] load server identity failed: %v", err)
	} else if discoveryManager, discoveryErr := discovery.StartContextSyncServiceManager(
		serverID,
		port,
	); discoveryErr != nil {
		log.Printf("[context-video-sync] mDNS manager start failed: %v", discoveryErr)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := discoveryManager.Shutdown(ctx); err != nil {
				log.Printf("[context-video-sync] mDNS shutdown timed out: %v", err)
			} else {
				log.Printf("[context-video-sync] mDNS shutdown finished")
			}
		}()
	}

	handler.ConfigureReverseLanRegistration(false, nil)
	if config.ReverseLanEnabled() {
		serverID, err := syncidentity.Current()
		if err != nil {
			log.Printf("[reverse-lan] load server identity failed: %v", err)
		} else {
			registry := reverselan.NewRegistry(
				serverID,
				reverselan.DefaultTicketTTL,
				reverselan.DefaultTicketCapacity,
			)
			reverseRuntime, startErr := reverselan.StartRuntime(serverID, port, registry)
			if startErr != nil {
				log.Printf("[reverse-lan] runtime start failed: %v", startErr)
			} else {
				handler.ConfigureReverseLanRegistration(true, registry)
				defer func() {
					handler.ConfigureReverseLanRegistration(false, nil)
					reverseRuntime.Shutdown()
					log.Printf("[reverse-lan] runtime shutdown finished")
				}()
				log.Printf("[reverse-lan] runtime started")
			}
		}
	}

	h := server.Default(
		server.WithHostPorts(net.JoinHostPort(config.ServerHost(), fmt.Sprint(port))),
		server.WithMaxRequestBodySize(int(config.ServerMaxRequestBodyBytes())),
		server.WithStreamBody(true),
	)

	// 简易 CORS 中间件：允许所有来源
	h.Use(cors())

	router.Register(h)
	h.SetCustomSignalWaiter(waitForServerShutdown)

	// #region debug-point D-E:http-engine-lifecycle
	reportDebugEvent("D,E", "main.go:before-spin", "[DEBUG] HTTP engine entering Spin", map[string]any{
		"pid": os.Getpid(), "host": config.ServerHost(), "port": port, "goroutines": runtime.NumGoroutine(),
	})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			reportDebugEvent("D,E", "main.go:heartbeat", "[DEBUG] server process heartbeat", map[string]any{
				"pid": os.Getpid(), "goroutines": runtime.NumGoroutine(),
			})
		}
	}()
	h.Spin()
	reportDebugEvent("D,E", "main.go:after-spin", "[DEBUG] HTTP engine Spin returned", map[string]any{
		"pid": os.Getpid(), "goroutines": runtime.NumGoroutine(),
	})
	// #endregion
}

func waitForServerShutdown(errCh chan error) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, serverShutdownSignals()...)
	defer signal.Stop(signals)
	select {
	case <-signals:
		return nil
	case err := <-errCh:
		return err
	}
}

func serverShutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

// #region debug-point A-B-D-E:reporter
var debugLogMu sync.Mutex

func reportDebugEvent(hypothesisID, location, message string, data map[string]any) {
	payload, err := json.Marshal(map[string]any{
		"sessionId": "zeronews-loopback-refused", "runId": "post-fix", "hypothesisId": hypothesisID,
		"location": location, "msg": message, "data": data, "ts": time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	workspace, err := os.Getwd()
	if err != nil {
		return
	}
	if filepath.Base(workspace) == "server" {
		workspace = filepath.Dir(workspace)
	}
	path := filepath.Join(workspace, ".dbg", "trae-debug-log-zeronews-loopback-refused.ndjson")
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o755); mkdirErr != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(payload, '\n'))
}

// #endregion

type rotatingLogWriter struct {
	mu        sync.Mutex
	path      string
	maxBytes  int64
	maxBackup int
}

func (writer *rotatingLogWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(writer.path), 0o755); err != nil {
		return 0, err
	}
	if info, err := os.Stat(writer.path); err == nil && info.Size()+int64(len(data)) > writer.maxBytes {
		_ = os.Remove(fmt.Sprintf("%s.%d", writer.path, writer.maxBackup))
		for index := writer.maxBackup - 1; index >= 1; index-- {
			_ = os.Rename(
				fmt.Sprintf("%s.%d", writer.path, index),
				fmt.Sprintf("%s.%d", writer.path, index+1),
			)
		}
		if err := os.Rename(writer.path, writer.path+".1"); err != nil {
			return 0, err
		}
	}
	file, err := os.OpenFile(writer.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return file.Write(data)
}

func enableServerLog() func() {
	path := config.ServerLogPath()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(io.MultiWriter(os.Stderr, &rotatingLogWriter{
		path:      path,
		maxBytes:  20 * 1024 * 1024,
		maxBackup: 4,
	}))
	return func() {
		log.SetOutput(os.Stderr)
	}
}

// cors 允许跨域访问（含预检请求）
func cors() app.HandlerFunc {
	allowedOrigins := config.CORSAllowedOrigins()
	return func(ctx context.Context, c *app.RequestContext) {
		origin := string(c.GetHeader("Origin"))
		if contains(allowedOrigins, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		if string(c.Method()) == consts.MethodOptions {
			c.AbortWithStatus(consts.StatusNoContent)
			return
		}
		c.Next(ctx)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
