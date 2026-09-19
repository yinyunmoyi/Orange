// Package dict 提供本地 ECDICT 词典查询能力。
//
// 数据文件为 stardict.db（SQLite），需要用户或部署脚本手动下载后放置到
//
//	data/ecdict/stardict.db
//
// 数据源：https://github.com/skywind3000/ECDICT/releases (ecdict-sqlite-*.zip)
//
// 该包实现只读访问。ECDICT 是服务的必需依赖，文件缺失、数据库损坏或空表
// 都会使服务启动失败；外部词典 API 仅处理 ECDICT 未收录的词条。
package dict

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ErrNotFound 表示 ECDICT 中没有该词条。上层可据此决定是否回退。
var ErrNotFound = errors.New("ecdict: word not found")

// EcdictEntry 直接映射 stardict 表的一行。字段命名与官方 CSV 保持一致。
type EcdictEntry struct {
	ID          int64  `gorm:"column:id;primaryKey"`
	Word        string `gorm:"column:word"`
	Sw          string `gorm:"column:sw"`
	Phonetic    string `gorm:"column:phonetic"`
	Definition  string `gorm:"column:definition"`  // 英文释义（多行 \n 分隔）
	Translation string `gorm:"column:translation"` // 中文释义
	Pos         string `gorm:"column:pos"`         // e.g. "n:35/v:20"
	Collins     int    `gorm:"column:collins"`     // 柯林斯星级 0-5
	Oxford      int    `gorm:"column:oxford"`      // 牛津核心 3000 标记 0/1
	Tag         string `gorm:"column:tag"`         // 空格分隔标签
	Bnc         int    `gorm:"column:bnc"`
	Frq         int    `gorm:"column:frq"`
	Exchange    string `gorm:"column:exchange"`
	Detail      string `gorm:"column:detail"`
	Audio       string `gorm:"column:audio"`
}

// TableName 显式指定表名，避免 GORM 复数化推断出错。
func (EcdictEntry) TableName() string { return "stardict" }

var (
	db       *gorm.DB
	enabled  bool
	initErr  error
	initOnce sync.Once
)

// Init 打开并校验 ECDICT 数据库。多次调用返回首次初始化结果。
func Init(dbPath string) error {
	initOnce.Do(func() {
		conn, err := gorm.Open(sqlite.Open(dbPath+"?mode=ro&_journal_mode=OFF"), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			initErr = fmt.Errorf("open ECDICT database %q: %w", dbPath, err)
			return
		}
		var count int64
		if err := conn.Table("stardict").Count(&count).Error; err != nil {
			initErr = fmt.Errorf("validate ECDICT stardict table: %w", err)
			return
		}
		if count <= 0 {
			initErr = errors.New("validate ECDICT stardict table: database is empty")
			return
		}
		db = conn
		enabled = true
		log.Printf("[ecdict] loaded path=%s entries=%d", dbPath, count)
	})
	return initErr
}

// Enabled 返回词典是否可用。上层查询前须先判断。
func Enabled() bool { return enabled }

// Lookup 按大小写不敏感精确匹配查一个词条。未命中返回 ErrNotFound。
func Lookup(ctx context.Context, word string) (*EcdictEntry, error) {
	if !enabled {
		return nil, ErrNotFound
	}
	w := strings.TrimSpace(word)
	if w == "" {
		return nil, ErrNotFound
	}
	t0 := time.Now()
	var entry EcdictEntry
	err := db.WithContext(ctx).
		Where("word = ? COLLATE NOCASE", w).
		Take(&entry).Error
	elapsed := time.Since(t0)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[ecdict] miss word=%q elapsed=%s", w, elapsed)
			return nil, ErrNotFound
		}
		log.Printf("[ecdict] query err word=%q err=%v elapsed=%s", w, err, elapsed)
		return nil, err
	}
	log.Printf("[ecdict] hit word=%q collins=%d oxford=%d tag=%q elapsed=%s",
		w, entry.Collins, entry.Oxford, entry.Tag, elapsed)
	return &entry, nil
}
