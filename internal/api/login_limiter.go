package api

import (
	"sync"
	"time"
)

type loginLimiter struct {
	mu      sync.Mutex
	items   map[string]loginAttempt
	max     int
	window  time.Duration
	lockFor time.Duration
}

type loginAttempt struct {
	failures int
	first    time.Time
	until    time.Time
}

func newLoginLimiter(max int, window, lockFor time.Duration) *loginLimiter {
	return &loginLimiter{items: make(map[string]loginAttempt), max: max, window: window, lockFor: lockFor}
}

func (l *loginLimiter) allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.items[key]
	if !ok {
		return true
	}
	now := time.Now()
	if !a.until.IsZero() && now.Before(a.until) {
		return false
	}
	if now.Sub(a.first) > l.window {
		delete(l.items, key)
		return true
	}
	return a.failures < l.max
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	a, ok := l.items[key]
	if !ok || now.Sub(a.first) > l.window {
		a = loginAttempt{first: now}
	}
	a.failures++
	if a.failures >= l.max {
		a.until = now.Add(l.lockFor)
	}
	l.items[key] = a
}

func (l *loginLimiter) success(key string) {
	l.mu.Lock()
	delete(l.items, key)
	l.mu.Unlock()
}

func (l *loginLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for key, a := range l.items {
		if now.After(a.until) && now.Sub(a.first) > l.window {
			delete(l.items, key)
		}
	}
}
