package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	MaxKeys    = 200
	MaxSlow    = 50
	KeyPattern = "*"
)

type Client struct {
	rdb     *goredis.Client
	timeout time.Duration
}

type Info struct {
	Version   string
	Mode      string
	Role      string
	Uptime    time.Duration
	Clients   int64
	UsedBytes int64
	UsedHuman string
	OpsPerSec int64
	HitRate   float64
	DBSize    int64
}

type SlowEntry struct {
	ID       int64
	At       time.Time
	Duration time.Duration
	Args     []string
}

type KeyEntry struct {
	Name    string
	Type    string
	TTLSecs int64
}

func ConnectLazy(addr, password string, db int, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("redis addr is empty")
	}
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &Client{rdb: rdb, timeout: timeout}, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.timeout)
}

func parseSection(payload string) map[string]string {
	fields := map[string]string{}
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[key] = value
	}
	return fields
}

func number(fields map[string]string, key string) int64 {
	n, _ := strconv.ParseInt(fields[key], 10, 64)
	return n
}

func hitRate(hits, misses int64) float64 {
	if hits+misses == 0 {
		return -1
	}
	return float64(hits) / float64(hits+misses) * 100
}

func (c *Client) Info(ctx context.Context) (*Info, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	payload, err := c.rdb.Info(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis info: %w", err)
	}
	size, err := c.rdb.DBSize(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis dbsize: %w", err)
	}
	fields := parseSection(payload)
	hits := number(fields, "keyspace_hits")
	misses := number(fields, "keyspace_misses")
	return &Info{
		Version:   fields["redis_version"],
		Mode:      fields["redis_mode"],
		Role:      fields["role"],
		Uptime:    time.Duration(number(fields, "uptime_in_seconds")) * time.Second,
		Clients:   number(fields, "connected_clients"),
		UsedBytes: number(fields, "used_memory"),
		UsedHuman: fields["used_memory_human"],
		OpsPerSec: number(fields, "instantaneous_ops_per_sec"),
		HitRate:   hitRate(hits, misses),
		DBSize:    size,
	}, nil
}

func (c *Client) Slowlog(ctx context.Context) ([]SlowEntry, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	entries, err := c.rdb.SlowLogGet(ctx, MaxSlow).Result()
	if err != nil {
		return nil, fmt.Errorf("redis slowlog: %w", err)
	}
	out := make([]SlowEntry, len(entries))
	for i, e := range entries {
		out[i] = SlowEntry{ID: e.ID, At: e.Time, Duration: e.Duration, Args: e.Args}
	}
	return out, nil
}

func (c *Client) Keys(ctx context.Context) ([]KeyEntry, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	var names []string
	iter := c.rdb.Scan(ctx, 0, KeyPattern, 100).Iterator()
	for iter.Next(ctx) && len(names) < MaxKeys {
		names = append(names, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("redis scan: %w", err)
	}
	pipe := c.rdb.Pipeline()
	types := make([]*goredis.StatusCmd, len(names))
	ttls := make([]*goredis.DurationCmd, len(names))
	for i, name := range names {
		types[i] = pipe.Type(ctx, name)
		ttls[i] = pipe.TTL(ctx, name)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("redis key meta: %w", err)
	}
	out := make([]KeyEntry, len(names))
	for i, name := range names {
		ttlSecs := int64(-2)
		if d, err := ttls[i].Result(); err == nil {
			ttlSecs = int64(d / time.Second)
		}
		out[i] = KeyEntry{Name: name, Type: types[i].Val(), TTLSecs: ttlSecs}
	}
	return out, nil
}
