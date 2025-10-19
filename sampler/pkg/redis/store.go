package redis

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	samplerv1 "github.com/kumarabd/ingestion-plane/contracts/sampler/v1"
	"github.com/redis/go-redis/v9"
)

// Store handles Redis state management for sampling
type Store struct {
	rdb *redis.Client
}

// New creates a new Redis store
func New(cfg *Config) *Store {
	return &Store{
		rdb: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
	}
}

// Ping checks Redis connectivity
func (s *Store) Ping(ctx context.Context) error {
	return s.rdb.Ping(ctx).Err()
}

// Close closes the Redis connection
func (s *Store) Close() error {
	return s.rdb.Close()
}

// Counts represents template occurrence counts and statistics
type Counts struct {
	CurMin uint64
	Win10m uint64
	Win1h  uint64
	Win24h uint64
	Total  uint64
	EMA    float64
	First  time.Time
	Last   time.Time
}

// Touch updates minute bucket, total, last/first seen, and EMA
func (s *Store) Touch(ctx context.Context, tenant, template, service, env string, sev samplerv1.Severity, ts time.Time, emaAlpha float64) (Counts, error) {
	var out Counts
	base := baseKey(tenant, template, service, env, sev)
	min := minute(ts)
	minKey := fmt.Sprintf("mb:%s:%d", base, min)

	log.Printf("DEBUG: Redis touch - base=%s, min=%d, minKey=%s", base, min, minKey)

	pipe := s.rdb.TxPipeline()
	// INCR minute bucket, set TTL ~26h
	mbIncr := pipe.IncrBy(ctx, minKey, 1)
	pipe.Expire(ctx, minKey, 26*time.Hour)

	// total++
	totalIncr := pipe.IncrBy(ctx, "total:"+base, 1)
	// set first if not exists
	pipe.SetNX(ctx, "first:"+base, ts.Unix(), 0)
	// set last
	pipe.Set(ctx, "last:"+base, ts.Unix(), 0)

	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Printf("ERROR: Redis pipeline exec failed: %v", err)
		return out, err
	}
	out.CurMin = uint64(mbIncr.Val())
	out.Total = uint64(totalIncr.Val())

	log.Printf("DEBUG: Redis pipeline executed - curMin=%d, total=%d", out.CurMin, out.Total)

	// read last/first
	pipe2 := s.rdb.TxPipeline()
	firstGet := pipe2.Get(ctx, "first:"+base)
	lastGet := pipe2.Get(ctx, "last:"+base)
	emaGet := pipe2.Get(ctx, "ema:"+base)
	_, _ = pipe2.Exec(ctx) // ignore error for missing keys

	out.First = parseUnix(firstGet.Val())
	out.Last = parseUnix(lastGet.Val())
	out.EMA = atofSafe(emaGet.Val())

	log.Printf("DEBUG: Read state - first=%v, last=%v, ema=%.6f", out.First, out.Last, out.EMA)

	// update EMA and write back
	newEMA := emaAlpha*float64(out.CurMin) + (1.0-emaAlpha)*out.EMA
	_ = s.rdb.Set(ctx, "ema:"+base, fmt.Sprintf("%.6f", newEMA), 0).Err()
	out.EMA = newEMA

	log.Printf("DEBUG: EMA updated - alpha=%.6f, oldEMA=%.6f, curMin=%d, newEMA=%.6f",
		emaAlpha, atofSafe(emaGet.Val()), out.CurMin, newEMA)

	// window sums: fetch last N minutes
	var win10 = 10
	var win60 = 60
	var win1440 = 24 * 60

	out.Win10m = s.sumWindow(ctx, base, min, win10)
	out.Win1h = s.sumWindow(ctx, base, min, win60)
	out.Win24h = s.sumWindow(ctx, base, min, win1440)

	log.Printf("DEBUG: Window sums calculated - 10m=%d, 1h=%d, 24h=%d", out.Win10m, out.Win1h, out.Win24h)
	return out, nil
}

// sumWindow sums counts for a time window
func (s *Store) sumWindow(ctx context.Context, base string, currentMinute int64, spanMins int) uint64 {
	pipe := s.rdb.Pipeline()
	cmds := make([]*redis.StringCmd, 0, spanMins+1)
	start := currentMinute - int64(spanMins)

	log.Printf("DEBUG: sumWindow - base=%s, currentMin=%d, spanMins=%d, start=%d",
		base, currentMinute, spanMins, start)

	for m := start; m <= currentMinute; m++ {
		key := fmt.Sprintf("mb:%s:%d", base, m)
		cmds = append(cmds, pipe.Get(ctx, key))
	}
	_, _ = pipe.Exec(ctx)
	var sum uint64
	var nonZeroCount int
	for _, c := range cmds {
		if c.Err() == nil {
			if v, err := strconv.ParseUint(c.Val(), 10, 64); err == nil {
				if v > 0 {
					nonZeroCount++
				}
				sum += v
			}
		}
	}

	log.Printf("DEBUG: sumWindow result - sum=%d, nonZeroBuckets=%d/%d", sum, nonZeroCount, len(cmds))
	return sum
}

// Helper functions

func minute(ts time.Time) int64 {
	return ts.UTC().Unix() / 60
}

func baseKey(tenant, template, service, env string, sev samplerv1.Severity) string {
	tenant = nz(tenant, "default")
	return fmt.Sprintf("sampler:%s:%s:%s:%s:%d", tenant, template, nz(service, "-"), nz(env, "-"), int(sev))
}

func nz(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func parseUnix(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	sec, _ := strconv.ParseInt(s, 10, 64)
	return time.Unix(sec, 0).UTC()
}

func atofSafe(s string) float64 {
	if s == "" {
		return 0.0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0
	}
	return f
}
