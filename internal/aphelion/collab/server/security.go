package server

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	CloseSlowConsumer websocket.StatusCode = 4408
	CloseRateLimited  websocket.StatusCode = 4429
)

type rateEntry struct {
	windowStarted time.Time
	lastSeen      time.Time
	count         int
}

type rateLimiter struct {
	mutex       sync.Mutex
	policy      RateLimit
	maxEntries  int
	entries     map[string]rateEntry
	lastCleanup time.Time
}

func newRateLimiter(policy RateLimit, maxEntries int) *rateLimiter {
	return &rateLimiter{policy: policy, maxEntries: maxEntries, entries: make(map[string]rateEntry)}
}

func (limiter *rateLimiter) Allow(key string, now time.Time) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.cleanup(now)
	entry, exists := limiter.entries[key]
	if !exists && len(limiter.entries) >= limiter.maxEntries {
		return false
	}
	if !exists || now.Sub(entry.windowStarted) >= limiter.policy.Window {
		limiter.entries[key] = rateEntry{windowStarted: now, lastSeen: now, count: 1}
		return true
	}
	entry.lastSeen = now
	entry.count++
	limiter.entries[key] = entry
	return entry.count <= limiter.policy.Burst
}

func (limiter *rateLimiter) cleanup(now time.Time) {
	if !limiter.lastCleanup.IsZero() && now.Sub(limiter.lastCleanup) < limiter.policy.Window {
		return
	}
	for key, entry := range limiter.entries {
		if now.Sub(entry.lastSeen) >= limiter.policy.Window {
			delete(limiter.entries, key)
		}
	}
	limiter.lastCleanup = now
}

func remoteIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}
