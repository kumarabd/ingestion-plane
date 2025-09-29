// cmd/sampler/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	samplerv1 "github.com/kumarabd/ingestion-plane/contracts/sampler/v1"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// ======================= Config =======================

type Config struct {
	GRPCPort         string
	RedisAddr        string
	RedisDB          int
	RedisPassword    string
	EMAHalfLifeMins  float64
	MinSpikePerMin   uint64
	SpikeFactor      float64
	WarmupN          uint64
	Log2Enabled      bool
	NoveltyWindow    time.Duration
	Version          string
	SteadyKDebug     uint64
	SteadyKInfo      uint64
	SteadyKWARN      uint64
	Budget10mDefault uint64 // simple per-key budget (optional floor)
}

func loadConfig() Config {
	return Config{
		GRPCPort:         getenv("GRPC_PORT", "50060"),
		RedisAddr:        getenv("REDIS_ADDR", "localhost:6379"),
		RedisDB:          atoi(getenv("REDIS_DB", "0")),
		RedisPassword:    os.Getenv("REDIS_PASSWORD"),
		EMAHalfLifeMins:  atof(getenv("EMA_HALFLIFE_MINS", "10")),
		MinSpikePerMin:   au64(getenv("MIN_SPIKE_PER_MIN", "100")),
		SpikeFactor:      atof(getenv("SPIKE_FACTOR", "3.0")),
		WarmupN:          au64(getenv("WARMUP_N", "32")),
		Log2Enabled:      getbool(getenv("LOG2_ENABLED", "true")),
		NoveltyWindow:    time.Duration(ai64(getenv("NOVELTY_WINDOW_MIN", "1440"))) * time.Minute, // 24h
		Version:          getenv("POLICY_VERSION", "sampler-redis-v1"),
		SteadyKDebug:     au64(getenv("STEADY_K_DEBUG", "500")),
		SteadyKInfo:      au64(getenv("STEADY_K_INFO", "100")),
		SteadyKWARN:      au64(getenv("STEADY_K_WARN", "10")),
		Budget10mDefault: au64(getenv("BUDGET_10M_DEFAULT", "0")), // 0 = disabled
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func atoi(s string) int     { i, _ := strconv.Atoi(strings.TrimSpace(s)); return i }
func ai64(s string) int64   { i, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64); return i }
func au64(s string) uint64  { u, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64); return u }
func atof(s string) float64 { f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return f }
func getbool(s string) bool { b, _ := strconv.ParseBool(strings.TrimSpace(s)); return b }

// ======================= Policy (local) =======================

type Policy struct {
	SeverityKeep     map[samplerv1.Severity]bool
	NoveltyWindow    time.Duration
	WarmupN          uint64
	Log2Enabled      bool
	SteadyK          map[samplerv1.Severity]uint64
	SpikeFactor      float64
	MinSpikePerMin   uint64
	EMAHalfLifeMins  float64
	Budget10mDefault uint64 // optional per-key 10m cap
	Version          string
}

func defaultPolicy(cfg Config) Policy {
	return Policy{
		SeverityKeep: map[samplerv1.Severity]bool{
			samplerv1.Severity_SEVERITY_ERROR: true,
			samplerv1.Severity_SEVERITY_FATAL: true,
			// WARN often sampled; flip to true if you want keep-all WARN
			samplerv1.Severity_SEVERITY_WARN: false,
		},
		NoveltyWindow:    cfg.NoveltyWindow,
		WarmupN:          cfg.WarmupN,
		Log2Enabled:      cfg.Log2Enabled,
		SteadyK:          map[samplerv1.Severity]uint64{samplerv1.Severity_SEVERITY_DEBUG: cfg.SteadyKDebug, samplerv1.Severity_SEVERITY_INFO: cfg.SteadyKInfo, samplerv1.Severity_SEVERITY_WARN: cfg.SteadyKWARN},
		SpikeFactor:      cfg.SpikeFactor,
		MinSpikePerMin:   cfg.MinSpikePerMin,
		EMAHalfLifeMins:  cfg.EMAHalfLifeMins,
		Budget10mDefault: cfg.Budget10mDefault,
		Version:          cfg.Version,
	}
}

// ======================= Redis state =======================

// Redis key layout (all UTC-minute resolution):
//  base = "sampler:{tenant}:{template}:{service}:{env}:{severity}"
//  Minute bucket counts:   mb:<base>:<minuteEpoch> -> INT (EXPIRE 26h)
//  Total seen:             total:<base>           -> INT
//  Last seen (unix sec):   last:<base>            -> INT
//  EMA per-minute rate:    ema:<base>             -> FLOAT (string)
//  First seen (unix sec):  first:<base>           -> INT (set if absent)

type store struct {
	rdb *redis.Client
	pol Policy
}

func newStore(cfg Config, pol Policy) *store {
	return &store{
		rdb: redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		}),
		pol: pol,
	}
}

type counts struct {
	curMin uint64
	win10m uint64
	win1h  uint64
	win24h uint64
	total  uint64
	ema    float64
	first  time.Time
	last   time.Time
}

func minute(ts time.Time) int64 { return ts.UTC().Unix() / 60 }

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

// touch updates minute bucket, total, last/first seen, and ema.
// It returns window sums by fetching the last N minute keys with pipelined MGET.
func (s *store) touch(ctx context.Context, tenant, template, service, env string, sev samplerv1.Severity, ts time.Time) (counts, error) {
	var out counts
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
	out.curMin = uint64(mbIncr.Val())
	out.total = uint64(totalIncr.Val())

	log.Printf("DEBUG: Redis pipeline executed - curMin=%d, total=%d", out.curMin, out.total)

	// read last/first
	pipe2 := s.rdb.TxPipeline()
	firstGet := pipe2.Get(ctx, "first:"+base)
	lastGet := pipe2.Get(ctx, "last:"+base)
	emaGet := pipe2.Get(ctx, "ema:"+base)
	_, _ = pipe2.Exec(ctx) // ignore error for missing keys

	out.first = parseUnix(firstGet.Val())
	out.last = parseUnix(lastGet.Val())
	out.ema = atofSafe(emaGet.Val())

	log.Printf("DEBUG: Read state - first=%v, last=%v, ema=%.6f", out.first, out.last, out.ema)

	// update EMA and write back
	alpha := emaAlphaFromHalfLifeMins(s.pol.EMAHalfLifeMins)
	newEMA := alpha*float64(out.curMin) + (1.0-alpha)*out.ema
	_ = s.rdb.Set(ctx, "ema:"+base, fmt.Sprintf("%.6f", newEMA), 0).Err()
	out.ema = newEMA

	log.Printf("DEBUG: EMA updated - alpha=%.6f, oldEMA=%.6f, curMin=%d, newEMA=%.6f",
		alpha, atofSafe(emaGet.Val()), out.curMin, newEMA)

	// window sums: fetch last N minutes
	var win10 = 10
	var win60 = 60
	var win1440 = 24 * 60

	out.win10m = s.sumWindow(ctx, base, min, win10)
	out.win1h = s.sumWindow(ctx, base, min, win60)
	out.win24h = s.sumWindow(ctx, base, min, win1440)

	log.Printf("DEBUG: Window sums calculated - 10m=%d, 1h=%d, 24h=%d", out.win10m, out.win1h, out.win24h)
	return out, nil
}

func (s *store) sumWindow(ctx context.Context, base string, currentMinute int64, spanMins int) uint64 {
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

func emaAlphaFromHalfLifeMins(hl float64) float64 {
	if hl <= 0 {
		return 0.2
	}
	// alpha = 1 - 0.5^(1/hl)
	return 1.0 - math.Pow(0.5, 1.0/hl)
}

func isPowerOfTwo(n uint64) bool { return n != 0 && (n&(n-1)) == 0 }

// ======================= Sampler gRPC =======================

type server struct {
	samplerv1.UnimplementedSamplerServiceServer
	cfg Config
	pol Policy
	st  *store
}

func newServer(cfg Config) *server {
	pol := defaultPolicy(cfg)
	st := newStore(cfg, pol)
	return &server{cfg: cfg, pol: pol, st: st}
}

func (s *server) Decide(ctx context.Context, req *samplerv1.DecisionRequest) (*samplerv1.Decision, error) {
	resp, err := s.DecideBatch(ctx, &samplerv1.DecisionBatchRequest{Items: []*samplerv1.DecisionRequest{req}})
	if err != nil {
		return nil, err
	}
	return resp.Items[0], nil
}

func (s *server) DecideBatch(ctx context.Context, req *samplerv1.DecisionBatchRequest) (*samplerv1.DecisionBatchResponse, error) {
	if req == nil || len(req.Items) == 0 {
		log.Printf("DEBUG: Empty decision batch received")
		return &samplerv1.DecisionBatchResponse{Items: nil}, nil
	}

	log.Printf("DEBUG: Processing decision batch with %d items", len(req.Items))
	out := make([]*samplerv1.Decision, 0, len(req.Items))
	now := time.Now().UTC()

	for i, it := range req.Items {
		if it == nil {
			log.Printf("ERROR: nil item at index %d", i)
			return nil, errors.New("nil item")
		}

		tenant := nz(it.TenantId, "default")
		svc := strings.TrimSpace(it.Labels["service"])
		env := strings.TrimSpace(it.Labels["env"])
		sev := it.Severity

		log.Printf("DEBUG: Processing item %d: tenant=%s, template_id=%s, service=%s, env=%s, severity=%s",
			i+1, tenant, it.TemplateId, svc, env, sev.String())

		ts := now
		if it.Timestamp != nil && it.Timestamp.Seconds != 0 {
			ts = time.Unix(it.Timestamp.Seconds, int64(it.Timestamp.Nanos)).UTC()
			log.Printf("DEBUG: Using provided timestamp: %v", ts)
		} else {
			log.Printf("DEBUG: Using current timestamp: %v", ts)
		}

		// Touch Redis counters/state
		log.Printf("DEBUG: Touching Redis state for template %s", it.TemplateId)
		cnt, err := s.st.touch(ctx, tenant, it.TemplateId, svc, env, sev, ts)
		if err != nil {
			log.Printf("ERROR: Redis touch failed for item %d: %v", i+1, err)
			return nil, fmt.Errorf("redis touch: %w", err)
		}

		log.Printf("DEBUG: Redis state updated - curMin=%d, win10m=%d, win1h=%d, win24h=%d, total=%d, ema=%.2f",
			cnt.curMin, cnt.win10m, cnt.win1h, cnt.win24h, cnt.total, cnt.ema)

		// Evaluate policy ladder
		action, reason, sampleRate, note := s.eval(tenant, sev, cnt)

		log.Printf("DEBUG: Policy evaluation result - action=%s, reason=%s, sampleRate=%d, note='%s'",
			action.String(), reason.String(), sampleRate, note)

		out = append(out, &samplerv1.Decision{
			RecordIndex:   it.RecordIndex,
			Action:        action,
			KeepReason:    reason,
			Counters:      &samplerv1.WindowCounts{Count_10M: cnt.win10m, Count_1H: cnt.win1h, Count_24H: cnt.win24h},
			SampleRate:    sampleRate,
			PolicyVersion: s.pol.Version,
			Note:          note,
		})
	}

	log.Printf("DEBUG: Decision batch processing complete: %d decisions generated", len(out))
	return &samplerv1.DecisionBatchResponse{Items: out}, nil
}

func (s *server) eval(tenant string, sev samplerv1.Severity, c counts) (samplerv1.Action, samplerv1.KeepReason, uint32, string) {
	log.Printf("DEBUG: Evaluating policy for severity=%s, total=%d, curMin=%d, ema=%.2f",
		sev.String(), c.total, c.curMin, c.ema)

	// 1) Severity floor
	if s.pol.SeverityKeep[sev] {
		log.Printf("DEBUG: Severity floor matched - keeping due to severity")
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_SEVERITY, 1, ""
	}

	now := c.last

	// 2) Novelty (first seen or unseen beyond window)
	if s.pol.NoveltyWindow > 0 {
		// first occurrence
		if c.total <= 1 {
			log.Printf("DEBUG: Novelty window matched - keeping due to first occurrence")
			return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_NOVEL, 1, "first-occurrence"
		}
		// If you want "unseen for N", you would store a separate per-template lastSeen in Redis and compare here.
		_ = now // placeholder, lastSeen already in counts
	}

	// 3) Spike relaxation
	if c.ema > 0 && float64(c.curMin) >= s.pol.SpikeFactor*c.ema && c.curMin >= s.pol.MinSpikePerMin {
		log.Printf("DEBUG: Spike detected - curMin=%d, ema=%.2f, factor=%.2f, minSpike=%d",
			c.curMin, c.ema, s.pol.SpikeFactor, s.pol.MinSpikePerMin)
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_SPIKE, 1, fmt.Sprintf("cur=%d ema=%.2f", c.curMin, c.ema)
	}

	// 4) Warmup
	if c.total <= s.pol.WarmupN {
		log.Printf("DEBUG: Warmup phase - total=%d/%d", c.total, s.pol.WarmupN)
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_WARMUP, 1, fmt.Sprintf("n=%d/%d", c.total, s.pol.WarmupN)
	}

	// 5) Log2 milestones
	if s.pol.Log2Enabled && isPowerOfTwo(c.total) {
		log.Printf("DEBUG: Log2 milestone - total=%d is power of 2", c.total)
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_LOG2, 1, fmt.Sprintf("n=%d", c.total)
	}

	// 6) Budget (simple per-key budget if enabled)
	if s.pol.Budget10mDefault > 0 && c.win10m > s.pol.Budget10mDefault &&
		(sev == samplerv1.Severity_SEVERITY_DEBUG || sev == samplerv1.Severity_SEVERITY_INFO) {
		k := s.steadyK(sev)
		log.Printf("DEBUG: Budget check - win10m=%d > default=%d, k=%d", c.win10m, s.pol.Budget10mDefault, k)
		if c.total%k == 0 {
			log.Printf("DEBUG: Budget exceeded but keep due to steadyK milestone")
			return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_BUDGET, uint32(k), fmt.Sprintf("budget>10m: %d>%d", c.win10m, s.pol.Budget10mDefault)
		}
		log.Printf("DEBUG: Budget exceeded - suppressing")
		return samplerv1.Action_ACTION_SUPPRESS, samplerv1.KeepReason_KEEP_REASON_BUDGET, uint32(k), fmt.Sprintf("budget>10m: %d>%d", c.win10m, s.pol.Budget10mDefault)
	}

	// 7) SteadyK floor
	k := s.steadyK(sev)
	log.Printf("DEBUG: SteadyK check - total=%d, k=%d, total%%k=%d", c.total, k, c.total%k)
	if c.total%k == 0 {
		log.Printf("DEBUG: SteadyK milestone - keeping")
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_STEADYK, uint32(k), ""
	}
	log.Printf("DEBUG: SteadyK - suppressing")
	return samplerv1.Action_ACTION_SUPPRESS, samplerv1.KeepReason_KEEP_REASON_STEADYK, uint32(k), ""
}

func (s *server) steadyK(sev samplerv1.Severity) uint64 {
	k := s.pol.SteadyK[sev]
	if k == 0 {
		k = 100
	}
	return k
}

// ======================= gRPC Boot =======================

func main() {
	log.Printf("INFO: Starting SamplerService")

	cfg := loadConfig()
	log.Printf("INFO: Configuration loaded:")
	log.Printf("  GRPC Port: %s", cfg.GRPCPort)
	log.Printf("  Redis Addr: %s", cfg.RedisAddr)
	log.Printf("  Redis DB: %d", cfg.RedisDB)
	log.Printf("  EMA Half-life: %.2f mins", cfg.EMAHalfLifeMins)
	log.Printf("  Min Spike Per Min: %d", cfg.MinSpikePerMin)
	log.Printf("  Spike Factor: %.2f", cfg.SpikeFactor)
	log.Printf("  Warmup N: %d", cfg.WarmupN)
	log.Printf("  Log2 Enabled: %t", cfg.Log2Enabled)
	log.Printf("  Novelty Window: %v", cfg.NoveltyWindow)
	log.Printf("  Version: %s", cfg.Version)
	log.Printf("  SteadyK Debug: %d", cfg.SteadyKDebug)
	log.Printf("  SteadyK Info: %d", cfg.SteadyKInfo)
	log.Printf("  SteadyK Warn: %d", cfg.SteadyKWARN)
	log.Printf("  Budget 10m Default: %d", cfg.Budget10mDefault)

	srv := newServer(cfg)

	// quick ping to Redis
	log.Printf("DEBUG: Pinging Redis...")
	if err := srv.st.rdb.Ping(context.Background()).Err(); err != nil {
		log.Printf("ERROR: Redis ping failed: %v", err)
		log.Fatalf("redis ping failed: %v", err)
	}
	log.Printf("INFO: Redis connection established")

	gs := grpc.NewServer(
		grpc.MaxRecvMsgSize(16*1024*1024),
		grpc.MaxSendMsgSize(16*1024*1024),
	)
	samplerv1.RegisterSamplerServiceServer(gs, srv)
	log.Printf("DEBUG: SamplerService gRPC server registered")

	hs := health.NewServer()
	healthpb.RegisterHealthServer(gs, hs)
	log.Printf("DEBUG: Health service registered")

	l, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Printf("ERROR: Failed to listen on port %s: %v", cfg.GRPCPort, err)
		log.Fatalf("listen: %v", err)
	}
	log.Printf("INFO: SamplerService (Redis-backed) listening on :%s", cfg.GRPCPort)
	if err := gs.Serve(l); err != nil {
		log.Printf("ERROR: gRPC server failed: %v", err)
		log.Fatalf("serve: %v", err)
	}
}
