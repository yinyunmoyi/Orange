package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"aaa_word/biz/config"
	"aaa_word/biz/db"
	"aaa_word/biz/dict"
	"aaa_word/biz/discovery"
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

	closeServerLog := enableServerLog()
	defer closeServerLog()

	db.Init()
	defer db.Close()

	if err := dict.Init(config.ECDICTPath()); err != nil {
		log.Fatalf("[ecdict] required dictionary unavailable: %v", err)
	}

	if serverID, err := syncidentity.Current(); err != nil {
		log.Printf("[context-video-sync] load server identity failed: %v", err)
	} else if discoveryServer, discoveryErr := discovery.StartContextSyncService(
		serverID,
		port,
	); discoveryErr != nil {
		log.Printf("[context-video-sync] mDNS registration failed: %v", discoveryErr)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := discovery.ShutdownContextSyncService(ctx, discoveryServer); err != nil {
				log.Printf("[context-video-sync] mDNS shutdown timed out: %v", err)
			} else {
				log.Printf("[context-video-sync] mDNS shutdown finished")
			}
		}()
		log.Printf("[context-video-sync] mDNS registered service=%s port=%d",
			discovery.ContextSyncServiceType, port)
	}

	h := server.Default(
		server.WithHostPorts(net.JoinHostPort(config.ServerHost(), fmt.Sprint(port))),
		server.WithMaxRequestBodySize(int(config.ServerMaxRequestBodyBytes())),
		server.WithStreamBody(true),
	)

	// 简易 CORS 中间件：允许所有来源
	h.Use(cors())

	router.Register(h)

	h.Spin()
}

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
