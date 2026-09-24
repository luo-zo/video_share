//go:build integration

package database

import (
	"context"
	"database/sql"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"video_share/internal/testutil"
)

// This exercises the documented logical-backup path against two databases
// created exclusively by testutil. Neither the application's database nor a
// user-supplied schema is ever dropped by this test.
func TestAccountAndVideoMetadataBackupRestore(t *testing.T) {
	if _, err := exec.LookPath("mysqldump"); err != nil {
		missingBackupTool(t, err)
	}
	if _, err := exec.LookPath("mysql"); err != nil {
		missingBackupTool(t, err)
	}
	sourceDSN := testutil.MySQLDSN(t)
	restoreDSN := testutil.MySQLDSN(t)
	sourceConfig, err := mysql.ParseDSN(sourceDSN)
	if err != nil {
		t.Fatal(err)
	}
	restoreConfig, err := mysql.ParseDSN(restoreDSN)
	if err != nil {
		t.Fatal(err)
	}
	if sourceConfig.Net != "tcp" || restoreConfig.Net != "tcp" || sourceConfig.Addr != restoreConfig.Addr || sourceConfig.User != restoreConfig.User {
		t.Fatal("backup smoke test requires two isolated databases on the same TCP MySQL server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	source, err := sql.Open("mysql", sourceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	migrator, err := NewMigrator(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatalf("migrate isolated source: %v", err)
	}

	suffix := uuid.NewString()
	username := "backup_" + strings.ReplaceAll(suffix[:12], "-", "")
	objectKey := "backup/" + suffix + ".mp4"
	if _, err := source.ExecContext(ctx, `INSERT INTO users (username,password_hash,nickname,role) VALUES (?, 'synthetic-hash', 'Backup Test', 'user')`, username); err != nil {
		t.Fatal(err)
	}
	if _, err := source.ExecContext(ctx, `INSERT INTO videos (user_id,title,description,object_key,status,visibility,file_size,content_type)
		SELECT id,'Backup Video','',?,2,1,1,'video/mp4' FROM users WHERE username=?`, objectKey, username); err != nil {
		t.Fatal(err)
	}

	host, port, err := net.SplitHostPort(sourceConfig.Addr)
	if err != nil {
		t.Fatalf("unsupported MySQL TCP address: %v", err)
	}
	hostArgs := []string{"--protocol=tcp", "--host=" + host, "--port=" + port, "--user=" + sourceConfig.User}
	clientEnv := append(os.Environ(), "MYSQL_PWD="+sourceConfig.Passwd)
	backupPath := filepath.Join(t.TempDir(), "metadata.sql")
	dumpArgs := append([]string{"--single-transaction", "--no-tablespaces", "--default-character-set=utf8mb4", "--result-file=" + backupPath}, hostArgs...)
	dumpArgs = append(dumpArgs, sourceConfig.DBName)
	dump := exec.CommandContext(ctx, "mysqldump", dumpArgs...)
	dump.Env = clientEnv
	if output, err := dump.CombinedOutput(); err != nil {
		t.Fatalf("mysqldump isolated source: %v: %s", err, output)
	}
	backup, err := os.Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	restoreArgs := append(hostArgs, "--database="+restoreConfig.DBName, "--default-character-set=utf8mb4")
	restore := exec.CommandContext(ctx, "mysql", restoreArgs...)
	restore.Env = clientEnv
	restore.Stdin = backup
	if output, err := restore.CombinedOutput(); err != nil {
		t.Fatalf("restore into second isolated database: %v: %s", err, output)
	}

	restored, err := sql.Open("mysql", restoreDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var users, videos, migrations int
	if err := restored.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM users WHERE username=?),
		(SELECT COUNT(*) FROM videos WHERE object_key=? AND user_id=(SELECT id FROM users WHERE username=?)),
		(SELECT COUNT(*) FROM goose_db_version WHERE is_applied=1)`, username, objectKey, username).Scan(&users, &videos, &migrations); err != nil {
		t.Fatal(err)
	}
	if users != 1 || videos != 1 || migrations < 10 {
		t.Fatalf("restored users=%d videos=%d applied migrations=%d", users, videos, migrations)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("isolated backup restored: users=%d videos=%d applied migrations=%d backup bytes=%d", users, videos, migrations, info.Size())
}

func missingBackupTool(t *testing.T, err error) {
	t.Helper()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("REQUIRE_INTEGRATION_TESTS")), "true") {
		t.Fatalf("backup/restore client binaries are required: %v", err)
	}
	t.Skipf("backup/restore client binaries unavailable: %v", err)
}
