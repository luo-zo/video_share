package testutil

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// RedisTestConfig describes an isolated Redis database for integration tests.
// TEST_REDIS_DB defaults to 15 in optional mode so tests do not accidentally
// use the application's default DB 0. In required mode it must be explicit.
type RedisTestConfig struct {
	Addr     string
	Password string
	DB       int
}

// RedisConfig returns the Redis endpoint used by integration tests. Ordinary
// developer runs skip when Redis is not configured; CI or release gates can
// set REQUIRE_INTEGRATION_TESTS=true to fail closed instead.
func RedisConfig(t *testing.T) RedisTestConfig {
	t.Helper()
	addr := strings.TrimSpace(os.Getenv("TEST_REDIS_ADDR"))
	required := isTruthy(os.Getenv("REQUIRE_INTEGRATION_TESTS"))
	if addr == "" {
		if required {
			t.Fatal("TEST_REDIS_ADDR must be set when REQUIRE_INTEGRATION_TESTS=true")
		}
		t.Skip("TEST_REDIS_ADDR not set; skipping Redis integration test")
	}

	dbText := strings.TrimSpace(os.Getenv("TEST_REDIS_DB"))
	if dbText == "" {
		if required {
			t.Fatal("TEST_REDIS_DB must be set to a dedicated database when REQUIRE_INTEGRATION_TESTS=true")
		}
		dbText = "15"
	}
	db, err := strconv.Atoi(dbText)
	if err != nil || db < 0 {
		t.Fatalf("TEST_REDIS_DB must be a non-negative integer, got %q", dbText)
	}
	return RedisTestConfig{
		Addr:     addr,
		Password: os.Getenv("TEST_REDIS_PASSWORD"),
		DB:       db,
	}
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
