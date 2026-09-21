package agentapi

import (
	"sync"

	"golang.org/x/time/rate"
)

// driftRateLimiterInterval/Burst bound how often emitLocalAttrsMatchDrift's
// expensive body (org-wide pipeline scan + one audit_log write per flipped
// pipeline) can run per collector. 1 event per 30s with burst 1 matches the
// documented poll interval floor (collectors poll every 30-60s) -- a
// collector polling at any real-world cadence never gets rate-limited, only
// one reporting attributes that change faster than any real poll interval
// (a flapping pod IP, a version string mid-rollout) does. See PR-144 review
// §8: fixing §1 (make an attribute change actually dirty the serve cache)
// without this makes a flapping attribute trigger the full recompute path,
// not just this audit-log path, on every single poll -- shipped together.
const (
	driftRateLimiterInterval = rate.Limit(1.0 / 30.0) // 1 event per 30s
	driftRateLimiterBurst    = 1
)

// driftRateLimiter is a per-collector token bucket gating
// emitLocalAttrsMatchDrift's expensive body, mirroring
// internal/mgmtapi/machine_ratelimit.go's saRateLimiter shape exactly (same
// lazy-creation, no-eviction tradeoffs already accepted there) but keyed by
// collector id instead of service-account id, and living in agentapi since
// mgmtapi's limiter is scoped to that package's machine-auth gate.
//
// Unlike saRateLimiter, this never denies the request it's called from --
// GetConfig must always continue to serve config regardless of the
// limiter's answer. A deny here only skips this poll's drift
// recomputation; the underlying local_attributes write and served config
// are unaffected.
type driftRateLimiter struct {
	mu   sync.Mutex
	byID map[string]*rate.Limiter
}

func newDriftRateLimiter() *driftRateLimiter {
	return &driftRateLimiter{byID: make(map[string]*rate.Limiter)}
}

func (l *driftRateLimiter) allow(collectorID string) bool {
	l.mu.Lock()
	lim, ok := l.byID[collectorID]
	if !ok {
		lim = rate.NewLimiter(driftRateLimiterInterval, driftRateLimiterBurst)
		l.byID[collectorID] = lim
	}
	l.mu.Unlock()
	return lim.Allow()
}
