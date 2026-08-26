package relay

import (
	"sync"
	"time"
)

type windowLimiter struct {
	mutex      sync.Mutex
	budget     int64
	window     time.Duration
	maxEntries int
	entries    map[string]windowEntry
}

type windowEntry struct {
	started time.Time
	used    int64
}

func newWindowLimiter(budget int64, window time.Duration, maxEntries int) *windowLimiter {
	return &windowLimiter{budget: budget, window: window, maxEntries: maxEntries, entries: make(map[string]windowEntry)}
}

func (limiter *windowLimiter) Allow(key string, cost int64, now time.Time) bool {
	if key == "" || cost <= 0 || cost > limiter.budget {
		return false
	}
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	entry, exists := limiter.entries[key]
	if exists && now.Sub(entry.started) >= limiter.window {
		delete(limiter.entries, key)
		exists = false
	}
	if !exists && len(limiter.entries) >= limiter.maxEntries {
		for currentKey, current := range limiter.entries {
			if now.Sub(current.started) >= limiter.window {
				delete(limiter.entries, currentKey)
			}
		}
		if len(limiter.entries) >= limiter.maxEntries {
			return false
		}
	}
	if !exists {
		entry = windowEntry{started: now}
	}
	if entry.used+cost > limiter.budget {
		return false
	}
	entry.used += cost
	limiter.entries[key] = entry
	return true
}
