package infrastructure

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"cf-multi-tunnel/backend/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

// InitDB opens the SQLite database with settings that survive a multi-tenant
// panel's concurrent writes: WAL for readers, a busy timeout so a writer waits
// instead of failing, and foreign keys on.
func InitDB(path string) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)

	level := gormLogger.Warn
	if os.Getenv("DEBUG") == "true" {
		level = gormLogger.Info
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:  gormLogger.Default.LogMode(level),
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// SQLite has a single writer. Keeping the pool small avoids lock contention
	// that would otherwise surface as "database is locked".
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if err := AutoMigrate(db); err != nil {
		return nil, err
	}
	slog.Info("database ready", "path", path)
	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.RefreshToken{},
		&models.CFAccount{},
		&models.CFZone{},
		&models.Tunnel{},
		&models.PublishedApp{},
		&models.AuditLog{},
		&models.Setting{},
	)
}

func CloseDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
