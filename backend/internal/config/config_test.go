package config

import (
	"testing"
	"time"
)

func validConfig() *Config {
	return &Config{
		HTTPAddr:              ":8080",
		MySQLHost:             "127.0.0.1",
		MySQLPort:             3306,
		MySQLUser:             "root",
		MySQLPassword:         "pw",
		MySQLDatabase:         "db",
		JWTSecret:             "a-valid-secret",
		JWTIssuer:             "video-share",
		JWTTTL:                time.Hour,
		AppOrigin:             "http://127.0.0.1:5173",
		SessionFamilyTTL:      30 * 24 * time.Hour,
		RefreshTokenTTL:       30 * 24 * time.Hour,
		RefreshConflictWindow: 10 * time.Second,
		RateLimitPerSecond:    5,
		RateLimitBurst:        10,
		RedisAddr:             "127.0.0.1:6379",
		RedisDB:               0,
		RedisDialTimeout:      3 * time.Second,
		RedisReadTimeout:      2 * time.Second,
		RedisWriteTimeout:     2 * time.Second,
		RedisCommandTimeout:   time.Second,
		LoginRateLimit:        10,
		LoginRateWindow:       time.Minute,
		RegisterRateLimit:     5,
		RegisterRateWindow:    10 * time.Minute,
		CommentRateLimit:      10,
		CommentRateWindow:     time.Minute,
		ReportRateLimit:       5,
		ReportRateWindow:      10 * time.Minute,
		MinIOEndpoint:         "127.0.0.1:9000",
		MinIOPublicEndpoint:   "127.0.0.1:9000",
		MinIOAccessKey:        "test-access",
		MinIOSecretKey:        "test-secret",
		MinIOBucket:           "video-share",
		UploadExpiry:          10 * time.Minute,
		PlayExpiry:            time.Hour,
		MaxVideoBytes:         500 * 1024 * 1024,
		KafkaBrokers:          []string{"127.0.0.1:9092"},
		KafkaTranscodeTopic:   "video.transcode.requested",
		KafkaConsumerGroup:    "video-transcoder-v1",
		KafkaClientID:         "video-share",
		KafkaPublishTimeout:   5 * time.Second,
		TranscodeMaxAttempts:  3,
		TranscodeRetryBase:    5 * time.Second,
		OutboxPollInterval:    time.Second,
		WorkerID:              "test-worker",
		FFmpegPath:            "ffmpeg",
		FFprobePath:           "ffprobe",
		TranscodeTempDir:      ".tmp/transcode",
		TranscodeTimeout:      30 * time.Minute,
	}
}

func TestLoadProcessingDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("JWT_SECRET", "config-test-not-a-production-secret")
	t.Setenv("MYSQL_PASSWORD", "test-password")
	for _, key := range []string{"KAFKA_BROKERS", "KAFKA_TRANSCODE_TOPIC", "KAFKA_CONSUMER_GROUP", "KAFKA_CLIENT_ID", "KAFKA_PUBLISH_TIMEOUT_MS", "TRANSCODE_MAX_ATTEMPTS", "TRANSCODE_RETRY_BASE_SECONDS", "OUTBOX_POLL_INTERVAL_MS", "WORKER_ID", "FFMPEG_PATH", "FFPROBE_PATH", "TRANSCODE_TEMP_DIR", "TRANSCODE_TIMEOUT_SECONDS"} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.KafkaBrokers) != 1 || cfg.KafkaBrokers[0] != "127.0.0.1:9092" {
		t.Fatalf("KafkaBrokers = %v", cfg.KafkaBrokers)
	}
	if cfg.TranscodeMaxAttempts != 3 || cfg.TranscodeRetryBase != 5*time.Second || cfg.OutboxPollInterval != time.Second || cfg.KafkaPublishTimeout != 5*time.Second || cfg.TranscodeTimeout != 30*time.Minute {
		t.Fatalf("unexpected processing defaults: %+v", cfg)
	}
}

func TestLoadRedisDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("JWT_SECRET", "config-test-not-a-production-secret")
	t.Setenv("MYSQL_PASSWORD", "test-password")
	for _, key := range []string{
		"REDIS_ADDR", "REDIS_PASSWORD", "REDIS_DB", "REDIS_DIAL_TIMEOUT_MS",
		"REDIS_READ_TIMEOUT_MS", "REDIS_WRITE_TIMEOUT_MS", "REDIS_COMMAND_TIMEOUT_MS",
		"LOGIN_RATE_LIMIT", "LOGIN_RATE_WINDOW_SECONDS", "REGISTER_RATE_LIMIT",
		"REGISTER_RATE_WINDOW_SECONDS", "COMMENT_RATE_LIMIT", "COMMENT_RATE_WINDOW_SECONDS",
		"REPORT_RATE_LIMIT", "REPORT_RATE_WINDOW_SECONDS",
	} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RedisAddr != "127.0.0.1:6379" || cfg.RedisDB != 0 || cfg.RedisCommandTimeout != time.Second {
		t.Fatalf("unexpected Redis defaults: %+v", cfg)
	}
	if cfg.LoginRateLimit != 10 || cfg.LoginRateWindow != time.Minute || cfg.RegisterRateLimit != 5 || cfg.RegisterRateWindow != 10*time.Minute {
		t.Fatalf("unexpected auth rate defaults: %+v", cfg)
	}
	if cfg.CommentRateLimit != 10 || cfg.CommentRateWindow != time.Minute || cfg.ReportRateLimit != 5 || cfg.ReportRateWindow != 10*time.Minute {
		t.Fatalf("unexpected interaction rate defaults: %+v", cfg)
	}
	if cfg.JWTTTL != 15*time.Minute || cfg.AppOrigin != "http://127.0.0.1:5173" || cfg.SessionFamilyTTL != 30*24*time.Hour || cfg.RefreshConflictWindow != 10*time.Second {
		t.Fatalf("unexpected session defaults: %+v", cfg)
	}
}

func TestValidateRedisSettings(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"missing address", func(c *Config) { c.RedisAddr = "" }},
		{"negative database", func(c *Config) { c.RedisDB = -1 }},
		{"zero command timeout", func(c *Config) { c.RedisCommandTimeout = 0 }},
		{"zero login limit", func(c *Config) { c.LoginRateLimit = 0 }},
		{"zero register window", func(c *Config) { c.RegisterRateWindow = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.change(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected Redis/rate configuration to be rejected")
			}
		})
	}
}

func TestValidateKafkaPublishTimeout(t *testing.T) {
	for _, tc := range []struct {
		name      string
		timeout   time.Duration
		wantValid bool
	}{
		{"zero", 0, false},
		{"upper boundary", 2 * time.Minute, true},
		{"over maximum", 121 * time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.KafkaPublishTimeout = tc.timeout
			err := cfg.Validate()
			if (err == nil) != tc.wantValid {
				t.Fatalf("Validate() error = %v, want valid=%v", err, tc.wantValid)
			}
		})
	}
}

func TestValidateOK(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequiresJWTSecret(t *testing.T) {
	c := validConfig()
	c.JWTSecret = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty JWT_SECRET")
	}
}

func TestValidateRejectsPlaceholderSecret(t *testing.T) {
	for _, s := range []string{"change-me", "secret", "your-secret-key"} {
		c := validConfig()
		c.JWTSecret = s
		if err := c.Validate(); err == nil {
			t.Errorf("expected error for placeholder secret %q", s)
		}
	}
}

func TestValidateRequiresMySQLPassword(t *testing.T) {
	c := validConfig()
	c.MySQLPassword = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty MYSQL_PASSWORD")
	}
}

func TestValidateRequiresHostAndDB(t *testing.T) {
	c := validConfig()
	c.MySQLHost = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty MYSQL_HOST")
	}
	c = validConfig()
	c.MySQLDatabase = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty MYSQL_DB")
	}
}

func TestValidatePositiveTTL(t *testing.T) {
	c := validConfig()
	c.JWTTTL = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for non-positive TTL")
	}
}

func TestValidateOriginAndCookieSecurity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		origin string
		secure bool
		valid  bool
	}{
		{"exact http origin", "http://127.0.0.1:5173", false, true},
		{"trailing slash rejected", "http://127.0.0.1:5173/", false, false},
		{"path rejected", "http://127.0.0.1:5173/app", false, false},
		{"secure requires https", "http://127.0.0.1:5173", true, false},
		{"secure https", "https://video.example", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.AppOrigin = tc.origin
			cfg.CookieSecure = tc.secure
			if got := cfg.Validate() == nil; got != tc.valid {
				t.Fatalf("Validate() = %v, want valid=%v", got, tc.valid)
			}
		})
	}
}

func TestValidateSessionTTLBoundaries(t *testing.T) {
	c := validConfig()
	c.SessionFamilyTTL = 31 * 24 * time.Hour
	if err := c.Validate(); err == nil {
		t.Fatal("expected session family TTL above 30 days to be rejected")
	}
	c = validConfig()
	c.RefreshTokenTTL = c.SessionFamilyTTL + time.Hour
	if err := c.Validate(); err == nil {
		t.Fatal("expected refresh TTL beyond family expiry to be rejected")
	}
}

func TestValidatePositiveRate(t *testing.T) {
	c := validConfig()
	c.RateLimitPerSecond = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for non-positive rate limit")
	}
	c = validConfig()
	c.RateLimitBurst = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for non-positive burst")
	}
}

func TestGetEnvHelpers(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	t.Setenv("TEST_FLOAT", "1.5")
	t.Setenv("TEST_SLICE", "a, b ,")
	t.Setenv("TEST_INT_BAD", "not-a-number")

	if got := getEnvInt("TEST_INT", 0); got != 42 {
		t.Fatalf("getEnvInt = %d, want 42", got)
	}
	if got := getEnvInt("TEST_INT_BAD", 7); got != 7 {
		t.Fatalf("getEnvInt bad = %d, want fallback 7", got)
	}
	if got := getEnvInt("TEST_MISSING", 7); got != 7 {
		t.Fatalf("getEnvInt missing = %d, want 7", got)
	}
	if got := getEnvFloat("TEST_FLOAT", 0); got != 1.5 {
		t.Fatalf("getEnvFloat = %v, want 1.5", got)
	}
	if got := getEnvFloat("TEST_MISSING", 2.5); got != 2.5 {
		t.Fatalf("getEnvFloat missing = %v, want 2.5", got)
	}
	s := getEnvSlice("TEST_SLICE")
	if len(s) != 2 || s[0] != "a" || s[1] != "b" {
		t.Fatalf("getEnvSlice = %v, want [a b]", s)
	}
	if s := getEnvSlice("TEST_MISSING"); s != nil {
		t.Fatalf("getEnvSlice missing = %v, want nil", s)
	}
	if got := getEnv("TEST_INT", "fb"); got != "42" {
		t.Fatalf("getEnv = %q, want 42", got)
	}
	if got := getEnv("TEST_MISSING", "fb"); got != "fb" {
		t.Fatalf("getEnv missing = %q, want fb", got)
	}
}

func TestValidateStorage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Config)
	}{
		{"endpoint scheme", func(c *Config) { c.MinIOEndpoint = "http://127.0.0.1:9000" }},
		{"endpoint path", func(c *Config) { c.MinIOEndpoint = "127.0.0.1:9000/videos" }},
		{"empty endpoint", func(c *Config) { c.MinIOEndpoint = "" }},
		{"public endpoint scheme", func(c *Config) { c.MinIOPublicEndpoint = "http://127.0.0.1:9000" }},
		{"empty public endpoint", func(c *Config) { c.MinIOPublicEndpoint = "" }},
		{"empty access key", func(c *Config) { c.MinIOAccessKey = "" }},
		{"empty secret key", func(c *Config) { c.MinIOSecretKey = "" }},
		{"empty bucket", func(c *Config) { c.MinIOBucket = "" }},
		{"zero upload expiry", func(c *Config) { c.UploadExpiry = 0 }},
		{"overlong play expiry", func(c *Config) { c.PlayExpiry = 8 * 24 * time.Hour }},
		{"zero max size", func(c *Config) { c.MaxVideoBytes = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.change(c)
			if err := c.Validate(); err == nil {
				t.Fatal("expected invalid storage configuration to fail")
			}
		})
	}
}

func TestLoadStorageDefaultsAndInvalidValues(t *testing.T) {
	// 隔离工作目录，避免读取开发者真实 .env。
	t.Chdir(t.TempDir())
	t.Setenv("JWT_SECRET", "config-test-not-a-production-secret")
	t.Setenv("MYSQL_PASSWORD", "test-password")
	for _, key := range []string{"MINIO_ENDPOINT", "MINIO_PUBLIC_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_BUCKET", "MINIO_USE_SSL", "UPLOAD_EXPIRY_SECONDS", "PLAY_EXPIRY_SECONDS", "MAX_VIDEO_BYTES"} {
		t.Setenv(key, "")
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.MinIOEndpoint != "127.0.0.1:9000" || c.MinIOPublicEndpoint != c.MinIOEndpoint || c.MinIOUseSSL || c.UploadExpiry != 10*time.Minute || c.PlayExpiry != time.Hour || c.MaxVideoBytes != 500*1024*1024 {
		t.Fatal("incorrect local storage defaults")
	}
	for _, tc := range []struct{ key, value string }{
		{"MINIO_USE_SSL", "yes-please"},
		{"COOKIE_SECURE", "yes-please"},
		{"UPLOAD_EXPIRY_SECONDS", "abc"},
		{"UPLOAD_EXPIRY_SECONDS", "9999999999999999999"},
		{"PLAY_EXPIRY_SECONDS", "604801"},
		{"MAX_VIDEO_BYTES", "-1"},
		{"MAX_VIDEO_BYTES", "500MB"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid environment value silently accepted")
			}
		})
	}
}
