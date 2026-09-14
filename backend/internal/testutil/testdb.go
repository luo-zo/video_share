// Package testutil 为需要真实 MySQL 服务器的集成测试提供共享辅助函数。测试通过
// `integration` 构建标签选择加入，并将 TEST_MYSQL_DSN 设置为服务器 DSN；这些
// 辅助函数会为每次运行创建一个隔离的数据库，并在清理时仅删除该数据库。
package testutil

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// MySQLDSN 连接到 TEST_MYSQL_DSN 指定的 MySQL 服务器，为本次测试运行创建一个
// 唯一命名的数据库，并返回指向它的 DSN。清理时会删除该数据库（并暴露任何失败）。
// 当 TEST_MYSQL_DSN 未设置时跳过测试。
func MySQLDSN(t *testing.T) string {
	t.Helper()

	base := os.Getenv("TEST_MYSQL_DSN")
	if base == "" {
		t.Skip("TEST_MYSQL_DSN not set; skipping integration test")
	}

	cfg, err := mysql.ParseDSN(base)
	if err != nil {
		t.Fatalf("parse TEST_MYSQL_DSN: %v", err)
	}

	// 连接时不指定默认数据库，以便创建和删除我们自己的数据库。
	adminCfg := *cfg
	adminCfg.DBName = ""
	admin, err := sql.Open("mysql", adminCfg.FormatDSN())
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	if err := admin.Ping(); err != nil {
		_ = admin.Close()
		t.Fatalf("ping admin connection: %v", err)
	}

	name := fmt.Sprintf("video_share_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatalf("create test database: %v", err)
	}

	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS `" + name + "`"); err != nil {
			t.Errorf("drop test database %q: %v", name, err)
		}
		if err := admin.Close(); err != nil {
			t.Errorf("close admin connection: %v", err)
		}
	})

	testCfg := *cfg
	testCfg.DBName = name
	return testCfg.FormatDSN()
}
