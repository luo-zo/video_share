package database

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newCaptureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, nil))
	return l, &buf
}

// newDryRunDB 基于一个虚假的 DSN 以 DryRun 模式构建 GORM 句柄，使查询回调
// 能够执行（构建 SQL 并调用 logger），而无需真实数据库。
func newDryRunDB(t *testing.T, l logger.Interface) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		SkipInitializeWithVersion: true,
		DSN:                       "test:test@tcp(127.0.0.1:1)/nodb?parseTime=True",
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               l,
	})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return db
}

func TestGormLoggerParamsFilter(t *testing.T) {
	l, _ := newCaptureLogger()
	gl := NewGormLogger(l)

	filter, ok := gl.(gorm.ParamsFilter)
	if !ok {
		t.Fatal("logger does not implement gorm.ParamsFilter")
	}
	sql, params := filter.ParamsFilter(context.Background(),
		"SELECT * FROM users WHERE password_hash = ?", "SENSITIVE")
	if !strings.Contains(sql, "?") {
		t.Fatalf("SQL template lost its placeholder: %q", sql)
	}
	if params != nil {
		t.Fatalf("bind params not stripped: %v", params)
	}
}

func TestGormLoggerDoesNotLeakSensitiveParams(t *testing.T) {
	const marker = "SENSITIVE_MARKER_7f3a9c2e"

	t.Run("error path", func(t *testing.T) {
		l, buf := newCaptureLogger()
		db := newDryRunDB(t, NewGormLogger(l))
		db.Callback().Query().After("gorm:query").Register("synthetic_error", func(db *gorm.DB) {
			db.AddError(errors.New("synthetic failure"))
		})

		var out struct{ PasswordHash string }
		res := db.Table("users").Where("password_hash = ?", marker).Find(&out)
		if res.Error == nil {
			t.Fatal("expected synthetic error to be set")
		}

		got := buf.String()
		if strings.Contains(got, marker) {
			t.Fatalf("error path leaked sensitive value:\n%s", got)
		}
		if !strings.Contains(got, "password_hash = ?") {
			t.Fatalf("error path lost parameterized template:\n%s", got)
		}
	})

	t.Run("slow path", func(t *testing.T) {
		l, buf := newCaptureLogger()
		// 负的阈值会让每条查询都确定性地被视为“慢查询”，
		// 无论 DryRun 查询实际花费多少墙上时钟时间。
		gl := &gormSlogLogger{l: l, slowThreshold: -time.Nanosecond}
		db := newDryRunDB(t, gl)

		var out struct{ PasswordHash string }
		db.Table("users").Where("password_hash = ?", marker).Find(&out)

		got := buf.String()
		if strings.Contains(got, marker) {
			t.Fatalf("slow path leaked sensitive value:\n%s", got)
		}
		if !strings.Contains(got, "password_hash = ?") {
			t.Fatalf("slow path lost parameterized template:\n%s", got)
		}
		if !strings.Contains(got, "slow db query") {
			t.Fatalf("expected slow query log:\n%s", got)
		}
	})
}
