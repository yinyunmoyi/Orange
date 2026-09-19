package db

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"aaa_word/biz/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// DB 全局 GORM 句柄；未配置 MYSQL_DSN 时为 nil
var DB *gorm.DB

// ErrDBDisabled 数据库未配置或初始化失败
var ErrDBDisabled = errors.New("db not configured")

// Init 尝试根据 .env 或环境变量中的 MYSQL_DSN 建立连接。
// 未配置或连接失败均不 fatal，仅日志警告，让收藏功能之外的接口保持可用。
func Init() {
	dsn := config.Get("MYSQL_DSN", "")
	if strings.TrimSpace(dsn) == "" {
		log.Printf("[db] MYSQL_DSN not configured, favorite feature disabled")
		return
	}

	gdb, err := Open(dsn)
	if err != nil {
		log.Printf("[db] mysql unavailable: %v", err)
		return
	}
	DB = gdb
	log.Printf("[db] mysql connected")
}

// Open 创建并校验数据库连接，供服务启动与显式迁移命令复用。
// 返回错误刻意不包装驱动错误，避免意外泄露 DSN 中的密码。
func Open(dsn string) (*gorm.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, ErrDBDisabled
	}
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, errors.New("open mysql connection failed")
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, errors.New("initialize mysql connection pool failed")
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, errors.New("ping mysql failed")
	}
	return gdb, nil
}

// Enabled 是否已成功建立数据库连接
func Enabled() bool {
	return DB != nil
}

// Close 关闭连接，供 main 退出前调用
func Close() {
	if err := CloseConnection(DB); err != nil {
		log.Printf("[db] close mysql: %v", err)
	}
}

func CloseConnection(gdb *gorm.DB) error {
	if gdb == nil {
		return nil
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return errors.New("get mysql connection pool failed")
	}
	return sqlDB.Close()
}

// FormatDSN 便捷函数（供开发调试打印），不暴露密码
func FormatDSN(host string, port int, user, dbName string) string {
	return fmt.Sprintf("%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4", user, host, port, dbName)
}
