package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Options controls the Redis connection and command timeouts.
type Options struct {
	Addr           string
	Password       string
	DB             int
	DialTimeout    time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	CommandTimeout time.Duration
}

// Client is the small Redis surface shared by cache and middleware. Every
// operation gets its own bounded command context so a broken Redis instance
// cannot hold an HTTP request forever.
type Client struct {
	client         *redis.Client
	commandTimeout time.Duration
}

const (
	defaultDialTimeout    = 3 * time.Second
	defaultReadTimeout    = 2 * time.Second
	defaultWriteTimeout   = 2 * time.Second
	defaultCommandTimeout = 1 * time.Second
)

// NewClient creates a Redis client without making a network call. Callers may
// choose to Ping explicitly during diagnostics; normal API startup keeps Redis
// optional so the documented per-feature degradation policies can apply.
func NewClient(options Options) *Client {
	if options.DialTimeout <= 0 {
		options.DialTimeout = defaultDialTimeout
	}
	if options.ReadTimeout <= 0 {
		options.ReadTimeout = defaultReadTimeout
	}
	if options.WriteTimeout <= 0 {
		options.WriteTimeout = defaultWriteTimeout
	}
	if options.CommandTimeout <= 0 {
		options.CommandTimeout = defaultCommandTimeout
	}
	return &Client{
		client: redis.NewClient(&redis.Options{
			Addr:         options.Addr,
			Password:     options.Password,
			DB:           options.DB,
			DialTimeout:  options.DialTimeout,
			ReadTimeout:  options.ReadTimeout,
			WriteTimeout: options.WriteTimeout,
		}),
		commandTimeout: options.CommandTimeout,
	}
}

// ErrMiss reports a cache miss. It is kept package-local to callers through
// IsMiss so they do not need to depend on the Redis client package directly.
var ErrMiss = redis.Nil

// IsMiss reports whether err means that a key was not present.
func IsMiss(err error) bool { return errors.Is(err, redis.Nil) }

func (c *Client) commandContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, c.commandTimeout)
}

// Ping verifies that the configured Redis endpoint is reachable.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("redis client is nil")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.Ping(commandCtx).Err()
}

// Get reads a string value.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	if c == nil || c.client == nil {
		return "", errors.New("redis client is nil")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.Get(commandCtx, key).Result()
}

// Set writes a value with a mandatory finite expiration.
func (c *Client) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	if c == nil || c.client == nil {
		return errors.New("redis client is nil")
	}
	if expiration <= 0 {
		return errors.New("redis expiration must be positive")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.Set(commandCtx, key, value, expiration).Err()
}

// Delete removes keys. It is safe to call after a successful MySQL write; a
// Redis failure is returned for logging/metrics but must not roll back MySQL.
func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if c == nil || c.client == nil {
		return errors.New("redis client is nil")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.Del(commandCtx, keys...).Err()
}

// ZMember is one sorted-set member used by ranking snapshots.
type ZMember struct {
	Score  float64
	Member string
}

// ZAdd replaces/updates sorted-set members. It is intentionally a small
// wrapper so feature packages do not depend on the concrete Redis client.
func (c *Client) ZAdd(ctx context.Context, key string, members ...ZMember) error {
	if c == nil || c.client == nil {
		return errors.New("redis client is nil")
	}
	if len(members) == 0 {
		return nil
	}
	values := make([]redis.Z, 0, len(members))
	for _, member := range members {
		values = append(values, redis.Z{Score: member.Score, Member: member.Member})
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.ZAdd(commandCtx, key, values...).Err()
}

// ZRevRange returns sorted-set members in score-descending order.
func (c *Client) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("redis client is nil")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.ZRevRange(commandCtx, key, start, stop).Result()
}

// ZRevRangeWithScores returns members and their snapshot scores together.
func (c *Client) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]ZMember, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("redis client is nil")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	values, err := c.client.ZRevRangeWithScores(commandCtx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}
	result := make([]ZMember, 0, len(values))
	for _, value := range values {
		member, ok := value.Member.(string)
		if !ok {
			member = fmt.Sprint(value.Member)
		}
		result = append(result, ZMember{Score: value.Score, Member: member})
	}
	return result, nil
}

// Expire applies a finite TTL to a key.
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) error {
	if c == nil || c.client == nil {
		return errors.New("redis client is nil")
	}
	if expiration <= 0 {
		return errors.New("redis expiration must be positive")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.Expire(commandCtx, key, expiration).Err()
}

// SetNX writes a lock or pointer only if the key does not exist.
func (c *Client) SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	if c == nil || c.client == nil {
		return false, errors.New("redis client is nil")
	}
	if expiration <= 0 {
		return false, errors.New("redis expiration must be positive")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.SetNX(commandCtx, key, value, expiration).Result()
}

// Rename atomically replaces a complete ranking key with its prepared temp
// key. Redis guarantees that readers observe either the old or the new set.
func (c *Client) Rename(ctx context.Context, source, destination string) error {
	if c == nil || c.client == nil {
		return errors.New("redis client is nil")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.Rename(commandCtx, source, destination).Err()
}

// RenameIfOwner atomically verifies a lock value and publishes a prepared key.
// A stale builder therefore cannot overwrite a newer snapshot after its lock
// has expired and been acquired by another process.
func (c *Client) RenameIfOwner(ctx context.Context, lockKey, source, destination, owner string) (bool, error) {
	if c == nil || c.client == nil {
		return false, errors.New("redis client is nil")
	}
	const script = `if redis.call('get', KEYS[1]) ~= ARGV[1] then return 0 end redis.call('rename', KEYS[2], KEYS[3]) return 1`
	result, err := c.Eval(ctx, script, []string{lockKey, source, destination}, owner)
	if err != nil {
		return false, err
	}
	switch value := result.(type) {
	case int64:
		return value == 1, nil
	case int:
		return value == 1, nil
	default:
		return false, fmt.Errorf("unexpected redis publish result %T", result)
	}
}

// Scan returns keys matching pattern without the blocking KEYS command. It is
// intended for bounded maintenance tasks such as pruning expired snapshots.
func (c *Client) Scan(ctx context.Context, pattern string, count int64) ([]string, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("redis client is nil")
	}
	if count <= 0 {
		count = 100
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	var cursor uint64
	var result []string
	for {
		keys, next, err := c.client.Scan(commandCtx, cursor, pattern, count).Result()
		if err != nil {
			return nil, err
		}
		result = append(result, keys...)
		cursor = next
		if cursor == 0 {
			return result, nil
		}
	}
}

// Eval executes an atomic Lua script.
func (c *Client) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("redis client is nil")
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	return c.client.Eval(commandCtx, script, keys, args...).Result()
}

// Close releases the underlying connection pool.
func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
