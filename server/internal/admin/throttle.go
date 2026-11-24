package admin

import (
	"sync"
	"time"
)

// throttle locks a key out for a while after too many failures inside a window.
type throttle struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	lockout  time.Duration
	now      func() time.Time
	attempts map[string]*attempt
}

type attempt struct {
	fails       int
	first       time.Time
	lockedUntil time.Time
}

func newThrottle(max int, window, lockout time.Duration) *throttle {
	return &throttle{max: max, window: window, lockout: lockout, now: time.Now, attempts: map[string]*attempt{}}
}

func (t *throttle) blocked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.attempts[key]
	return ok && t.now().Before(a.lockedUntil)
}

func (t *throttle) fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if len(t.attempts) > 1024 { // bound memory when many addresses probe
		for k, a := range t.attempts {
			if now.After(a.lockedUntil) && now.Sub(a.first) > t.window {
				delete(t.attempts, k)
			}
		}
	}
	a, ok := t.attempts[key]
	if !ok || now.Sub(a.first) > t.window {
		a = &attempt{first: now}
		t.attempts[key] = a
	}
	a.fails++
	if a.fails >= t.max {
		a.lockedUntil = now.Add(t.lockout)
		a.fails = 0
		a.first = now
	}
}

func (t *throttle) success(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if a, ok := t.attempts[key]; ok && !t.now().Before(a.lockedUntil) {
		delete(t.attempts, key)
	}
}
