package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"video_share/internal/config"
)

// DSN 构建 MySQL 数据源名称。密码仅保存在内存中使用。
func DSN(cfg *config.Config) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.MySQLUser, cfg.MySQLPassword, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDatabase)
}

// Open 连接 MySQL、应用连接池设置，并返回 *gorm.DB。
func Open(cfg *config.Config, l logger.Interface) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(DSN(cfg)), &gorm.Config{
		Logger: l,
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MySQLMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MySQLMaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.MySQLConnMaxLifetime)

	return db, nil
}

// Ping 在调用方的截止时间内验证数据库是否可达。
func Ping(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	return sqlDB.PingContext(ctx)
}

// defaultSlowQueryThreshold 是查询耗时阈值，超过该阈值的成功查询会被记录为
// 慢查询。
const defaultSlowQueryThreshold = 200 * time.Millisecond

// NewGormLogger 将 GORM 的 logger 适配为 slog。它只记录错误和慢查询，并实现
// gorm.ParamsFilter，使传给 Trace 的 SQL 是去掉所有绑定值后的参数化模板
// （'?' 占位符）——密码哈希及其他敏感值绝不会出现在日志中。
func NewGormLogger(l *slog.Logger) logger.Interface {
	return &gormSlogLogger{l: l, slowThreshold: defaultSlowQueryThreshold}
}

type gormSlogLogger struct {
	l             *slog.Logger
	slowThreshold time.Duration
}

// ParamsFilter 实现 gorm.ParamsFilter：保留 SQL 模板并丢弃所有绑定值。
// 这样 GORM 就不会进行插值，传给 Trace 的 SQL 只包含占位符而非字面值。
func (g *gormSlogLogger) ParamsFilter(_ context.Context, sql string, _ ...any) (string, []any) {
	return sql, nil
}

func (g *gormSlogLogger) LogMode(logger.LogLevel) logger.Interface { return g }

func (g *gormSlogLogger) Info(_ context.Context, msg string, data ...any) {
	g.l.Info(msg, "gorm", data)
}

func (g *gormSlogLogger) Warn(_ context.Context, msg string, data ...any) {
	g.l.Warn(msg, "gorm", data)
}

func (g *gormSlogLogger) Error(_ context.Context, msg string, data ...any) {
	g.l.Error(msg, "gorm", data)
}

func (g *gormSlogLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	if err != nil {
		sql, _ := fc()
		g.l.Error("db query error", "error", err, "sql", sql, "duration_ms", elapsed.Milliseconds())
		return
	}
	if elapsed > g.slowThreshold {
		sql, rows := fc()
		g.l.Warn("slow db query", "sql", sql, "rows", rows, "duration_ms", elapsed.Milliseconds())
	}
}
