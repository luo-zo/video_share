package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config 保存从环境变量加载的全部运行时配置。
type Config struct {
	HTTPAddr string

	MySQLHost     string
	MySQLPort     int
	MySQLUser     string
	MySQLPassword string
	MySQLDatabase string

	MySQLMaxOpenConns    int
	MySQLMaxIdleConns    int
	MySQLConnMaxLifetime time.Duration

	JWTSecret             string
	JWTIssuer             string
	JWTTTL                time.Duration
	AppOrigin             string
	CookieSecure          bool
	SessionFamilyTTL      time.Duration
	RefreshTokenTTL       time.Duration
	RefreshConflictWindow time.Duration

	TrustedProxies []string

	RateLimitPerSecond float64
	RateLimitBurst     int

	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	RedisDialTimeout    time.Duration
	RedisReadTimeout    time.Duration
	RedisWriteTimeout   time.Duration
	RedisCommandTimeout time.Duration

	LoginRateLimit     int
	LoginRateWindow    time.Duration
	RegisterRateLimit  int
	RegisterRateWindow time.Duration
	CommentRateLimit   int
	CommentRateWindow  time.Duration
	ReportRateLimit    int
	ReportRateWindow   time.Duration

	MinIOEndpoint       string
	MinIOPublicEndpoint string
	MinIOAccessKey      string
	MinIOSecretKey      string
	MinIOBucket         string
	MinIOUseSSL         bool
	UploadExpiry        time.Duration
	PlayExpiry          time.Duration
	MaxVideoBytes       int64

	KafkaBrokers        []string
	KafkaTranscodeTopic string
	KafkaConsumerGroup  string
	KafkaClientID       string
	KafkaPublishTimeout time.Duration

	TranscodeMaxAttempts int
	TranscodeRetryBase   time.Duration
	OutboxPollInterval   time.Duration
	WorkerID             string
	FFmpegPath           string
	FFprobePath          string
	TranscodeTempDir     string
	TranscodeTimeout     time.Duration
}

// Load 从环境变量读取配置。会先尝试加载工作目录下可选的 .env 文件，
// 缺少 .env 文件不算错误。
func Load() (*Config, error) {
	_ = godotenv.Load()
	cookieSecure, err := strconv.ParseBool(getEnv("COOKIE_SECURE", "false"))
	if err != nil {
		return nil, fmt.Errorf("COOKIE_SECURE must be a boolean")
	}

	cfg := &Config{
		HTTPAddr:              getEnv("HTTP_ADDR", "127.0.0.1:8081"),
		MySQLHost:             getEnv("MYSQL_HOST", "127.0.0.1"),
		MySQLPort:             getEnvInt("MYSQL_PORT", 3306),
		MySQLUser:             getEnv("MYSQL_USER", "root"),
		MySQLPassword:         os.Getenv("MYSQL_PASSWORD"),
		MySQLDatabase:         getEnv("MYSQL_DB", "video_share"),
		MySQLMaxOpenConns:     getEnvInt("MYSQL_MAX_OPEN_CONNS", 25),
		MySQLMaxIdleConns:     getEnvInt("MYSQL_MAX_IDLE_CONNS", 25),
		MySQLConnMaxLifetime:  time.Duration(getEnvInt("MYSQL_CONN_MAX_LIFETIME_SECONDS", 300)) * time.Second,
		JWTSecret:             os.Getenv("JWT_SECRET"),
		JWTIssuer:             getEnv("JWT_ISSUER", "video-share"),
		JWTTTL:                time.Duration(getEnvInt("JWT_TTL_SECONDS", 900)) * time.Second,
		AppOrigin:             getEnv("APP_ORIGIN", "http://127.0.0.1:5173"),
		CookieSecure:          cookieSecure,
		SessionFamilyTTL:      time.Duration(getEnvInt("SESSION_FAMILY_TTL_SECONDS", 30*24*60*60)) * time.Second,
		RefreshTokenTTL:       time.Duration(getEnvInt("REFRESH_TOKEN_TTL_SECONDS", 30*24*60*60)) * time.Second,
		RefreshConflictWindow: time.Duration(getEnvInt("REFRESH_CONFLICT_WINDOW_SECONDS", 10)) * time.Second,
		TrustedProxies:        getEnvSlice("TRUSTED_PROXIES"),
		RateLimitPerSecond:    getEnvFloat("RATE_LIMIT_PER_SECOND", 5),
		RateLimitBurst:        getEnvInt("RATE_LIMIT_BURST", 10),
		RedisAddr:             getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:         os.Getenv("REDIS_PASSWORD"),
		RedisDB:               getEnvInt("REDIS_DB", 0),
		RedisDialTimeout:      time.Duration(getEnvInt("REDIS_DIAL_TIMEOUT_MS", 3000)) * time.Millisecond,
		RedisReadTimeout:      time.Duration(getEnvInt("REDIS_READ_TIMEOUT_MS", 2000)) * time.Millisecond,
		RedisWriteTimeout:     time.Duration(getEnvInt("REDIS_WRITE_TIMEOUT_MS", 2000)) * time.Millisecond,
		RedisCommandTimeout:   time.Duration(getEnvInt("REDIS_COMMAND_TIMEOUT_MS", 1000)) * time.Millisecond,
		LoginRateLimit:        getEnvInt("LOGIN_RATE_LIMIT", 10),
		LoginRateWindow:       time.Duration(getEnvInt("LOGIN_RATE_WINDOW_SECONDS", 60)) * time.Second,
		RegisterRateLimit:     getEnvInt("REGISTER_RATE_LIMIT", 5),
		RegisterRateWindow:    time.Duration(getEnvInt("REGISTER_RATE_WINDOW_SECONDS", 600)) * time.Second,
		CommentRateLimit:      getEnvInt("COMMENT_RATE_LIMIT", 10),
		CommentRateWindow:     time.Duration(getEnvInt("COMMENT_RATE_WINDOW_SECONDS", 60)) * time.Second,
		ReportRateLimit:       getEnvInt("REPORT_RATE_LIMIT", 5),
		ReportRateWindow:      time.Duration(getEnvInt("REPORT_RATE_WINDOW_SECONDS", 600)) * time.Second,
		MinIOEndpoint:         getEnv("MINIO_ENDPOINT", "127.0.0.1:9000"),
		MinIOAccessKey:        getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:        getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOBucket:           getEnv("MINIO_BUCKET", "video-share"),
		KafkaBrokers:          getEnvSliceDefault("KAFKA_BROKERS", []string{"127.0.0.1:9092"}),
		KafkaTranscodeTopic:   getEnv("KAFKA_TRANSCODE_TOPIC", "video.transcode.requested"),
		KafkaConsumerGroup:    getEnv("KAFKA_CONSUMER_GROUP", "video-transcoder-v1"),
		KafkaClientID:         getEnv("KAFKA_CLIENT_ID", "video-share"),
		KafkaPublishTimeout:   time.Duration(getEnvInt("KAFKA_PUBLISH_TIMEOUT_MS", 5000)) * time.Millisecond,
		TranscodeMaxAttempts:  getEnvInt("TRANSCODE_MAX_ATTEMPTS", 3),
		TranscodeRetryBase:    time.Duration(getEnvInt("TRANSCODE_RETRY_BASE_SECONDS", 5)) * time.Second,
		OutboxPollInterval:    time.Duration(getEnvInt("OUTBOX_POLL_INTERVAL_MS", 1000)) * time.Millisecond,
		WorkerID:              getEnv("WORKER_ID", "video-worker-1"),
		FFmpegPath:            getEnv("FFMPEG_PATH", "ffmpeg"),
		FFprobePath:           getEnv("FFPROBE_PATH", "ffprobe"),
		TranscodeTempDir:      getEnv("TRANSCODE_TEMP_DIR", ".tmp/transcode"),
		TranscodeTimeout:      time.Duration(getEnvInt("TRANSCODE_TIMEOUT_SECONDS", 1800)) * time.Second,
	}
	cfg.MinIOPublicEndpoint = getEnv("MINIO_PUBLIC_ENDPOINT", cfg.MinIOEndpoint)
	cfg.MinIOUseSSL, err = strconv.ParseBool(getEnv("MINIO_USE_SSL", "false"))
	if err != nil {
		return nil, fmt.Errorf("MINIO_USE_SSL must be a boolean")
	}
	for _, setting := range []struct {
		name     string
		fallback string
		target   *time.Duration
	}{
		{"UPLOAD_EXPIRY_SECONDS", "600", &cfg.UploadExpiry},
		{"PLAY_EXPIRY_SECONDS", "3600", &cfg.PlayExpiry},
	} {
		seconds, parseErr := strconv.ParseInt(getEnv(setting.name, setting.fallback), 10, 64)
		if parseErr != nil || seconds < 1 || seconds > 7*24*60*60 {
			return nil, fmt.Errorf("%s must be between 1 and 604800", setting.name)
		}
		*setting.target = time.Duration(seconds) * time.Second
	}
	cfg.MaxVideoBytes, err = strconv.ParseInt(getEnv("MAX_VIDEO_BYTES", "524288000"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("MAX_VIDEO_BYTES must be a positive integer")
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate 检查必需的配置项，并对缺失或不安全的值快速失败。
// 它会主动拒绝空的和占位符形式的 JWT 密钥。
func (c *Config) Validate() error {
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required; generate one with: openssl rand -base64 32")
	}
	if c.JWTSecret == "change-me" || c.JWTSecret == "secret" || c.JWTSecret == "your-secret-key" {
		return fmt.Errorf("JWT_SECRET must not be a placeholder value")
	}
	if c.MySQLPassword == "" {
		return fmt.Errorf("MYSQL_PASSWORD is required")
	}
	if c.MySQLHost == "" {
		return fmt.Errorf("MYSQL_HOST is required")
	}
	if c.MySQLDatabase == "" {
		return fmt.Errorf("MYSQL_DB is required")
	}
	if c.JWTTTL <= 0 {
		return fmt.Errorf("JWT_TTL_SECONDS must be positive")
	}
	origin, err := url.Parse(c.AppOrigin)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.User != nil {
		return fmt.Errorf("APP_ORIGIN must be an exact http(s) origin without path or credentials")
	}
	if c.CookieSecure && origin.Scheme != "https" {
		return fmt.Errorf("COOKIE_SECURE=true requires an https APP_ORIGIN")
	}
	const maxSessionFamilyTTL = 30 * 24 * time.Hour
	if c.SessionFamilyTTL <= 0 || c.SessionFamilyTTL > maxSessionFamilyTTL || c.RefreshTokenTTL <= 0 || c.RefreshTokenTTL > maxSessionFamilyTTL || c.RefreshTokenTTL > c.SessionFamilyTTL || c.RefreshConflictWindow <= 0 || c.RefreshConflictWindow > c.RefreshTokenTTL {
		return fmt.Errorf("session TTLs and refresh conflict window are invalid (family/refresh max is 30 days)")
	}
	if c.RateLimitPerSecond <= 0 {
		return fmt.Errorf("RATE_LIMIT_PER_SECOND must be positive")
	}
	if c.RateLimitBurst <= 0 {
		return fmt.Errorf("RATE_LIMIT_BURST must be positive")
	}
	redisEndpoint, err := url.Parse("tcp://" + c.RedisAddr)
	if err != nil || redisEndpoint.Hostname() == "" || redisEndpoint.Port() == "" || redisEndpoint.Host != c.RedisAddr || redisEndpoint.Path != "" || redisEndpoint.RawQuery != "" || redisEndpoint.Fragment != "" || redisEndpoint.User != nil {
		return fmt.Errorf("REDIS_ADDR must be a hostname with port, without scheme or path")
	}
	if c.RedisDB < 0 || c.RedisDialTimeout <= 0 || c.RedisReadTimeout <= 0 || c.RedisWriteTimeout <= 0 || c.RedisCommandTimeout <= 0 {
		return fmt.Errorf("Redis database and timeouts must be non-negative/positive")
	}
	for name, rule := range map[string]struct {
		limit  int
		window time.Duration
	}{
		"LOGIN":    {c.LoginRateLimit, c.LoginRateWindow},
		"REGISTER": {c.RegisterRateLimit, c.RegisterRateWindow},
		"COMMENT":  {c.CommentRateLimit, c.CommentRateWindow},
		"REPORT":   {c.ReportRateLimit, c.ReportRateWindow},
	} {
		if rule.limit <= 0 || rule.window <= 0 {
			return fmt.Errorf("%s rate limit and window must be positive", name)
		}
	}
	for name, value := range map[string]string{
		"MINIO_ENDPOINT":        c.MinIOEndpoint,
		"MINIO_PUBLIC_ENDPOINT": c.MinIOPublicEndpoint,
	} {
		endpoint, err := url.Parse("http://" + value)
		if err != nil || endpoint.Hostname() == "" || endpoint.Host != value || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.User != nil {
			return fmt.Errorf("%s must be a hostname with optional port, without scheme or path", name)
		}
	}
	if strings.TrimSpace(c.MinIOAccessKey) == "" || strings.TrimSpace(c.MinIOSecretKey) == "" {
		return fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY are required")
	}
	if strings.TrimSpace(c.MinIOBucket) == "" {
		return fmt.Errorf("MINIO_BUCKET is required")
	}
	if c.UploadExpiry < time.Second || c.UploadExpiry > 7*24*time.Hour || c.PlayExpiry < time.Second || c.PlayExpiry > 7*24*time.Hour {
		return fmt.Errorf("upload and play expiry must be between one second and seven days")
	}
	if c.MaxVideoBytes <= 0 {
		return fmt.Errorf("MAX_VIDEO_BYTES must be positive")
	}
	if len(c.KafkaBrokers) == 0 {
		return fmt.Errorf("KAFKA_BROKERS is required")
	}
	for _, broker := range c.KafkaBrokers {
		endpoint, err := url.Parse("tcp://" + broker)
		if err != nil || endpoint.Hostname() == "" || endpoint.Port() == "" {
			return fmt.Errorf("KAFKA_BROKERS must contain host:port values")
		}
	}
	if strings.TrimSpace(c.KafkaTranscodeTopic) == "" || strings.TrimSpace(c.KafkaConsumerGroup) == "" || strings.TrimSpace(c.KafkaClientID) == "" {
		return fmt.Errorf("Kafka topic, consumer group and client ID are required")
	}
	if c.TranscodeMaxAttempts < 1 || c.TranscodeMaxAttempts > 20 {
		return fmt.Errorf("TRANSCODE_MAX_ATTEMPTS must be between 1 and 20")
	}
	if c.TranscodeRetryBase <= 0 || c.OutboxPollInterval <= 0 || c.TranscodeTimeout <= 0 {
		return fmt.Errorf("processing durations must be positive")
	}
	if c.KafkaPublishTimeout <= 0 || c.KafkaPublishTimeout > 2*time.Minute {
		return fmt.Errorf("KAFKA_PUBLISH_TIMEOUT_MS must be between 1 and 120000")
	}
	if strings.TrimSpace(c.WorkerID) == "" || strings.TrimSpace(c.FFmpegPath) == "" || strings.TrimSpace(c.FFprobePath) == "" || strings.TrimSpace(c.TranscodeTempDir) == "" {
		return fmt.Errorf("worker and FFmpeg configuration is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func getEnvSlice(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getEnvSliceDefault(key string, fallback []string) []string {
	values := getEnvSlice(key)
	if len(values) == 0 {
		return append([]string(nil), fallback...)
	}
	return values
}
