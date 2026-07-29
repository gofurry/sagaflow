package api

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

func parsePage(c fiber.Ctx, defaultPageSize, maxPageSize int) (int, int) {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", strconv.Itoa(defaultPageSize)))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func limitNonMultipartBody(limit int) fiber.Handler {
	return func(c fiber.Ctx) error {
		contentType := strings.ToLower(strings.TrimSpace(c.Get(fiber.HeaderContentType)))
		if !strings.HasPrefix(contentType, fiber.MIMEMultipartForm) {
			if size := c.Request().Header.ContentLength(); size > limit {
				return fiber.NewError(fiber.StatusRequestEntityTooLarge, "request body is too large")
			}
		}
		return c.Next()
	}
}

func (s *Server) withUploadSlot(handler fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		select {
		case s.uploadSlots <- struct{}{}:
			defer func() { <-s.uploadSlots }()
			return handler(c)
		default:
			c.Set(fiber.HeaderRetryAfter, "1")
			return fiber.NewError(fiber.StatusServiceUnavailable, "too many concurrent uploads")
		}
	}
}

type loginAttempt struct {
	windowStart time.Time
	failures    int
}

type loginRateLimiter struct {
	mu      sync.Mutex
	entries map[string]loginAttempt
	now     func() time.Time
	window  time.Duration
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		entries: make(map[string]loginAttempt),
		now:     time.Now,
		window:  time.Minute,
	}
}

func (l *loginRateLimiter) Allow(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	ipAttempt := l.current(now, "ip:"+ip)
	pairAttempt := l.current(now, "pair:"+ip+"\x00"+username)
	return ipAttempt.failures < 20 && pairAttempt.failures < 5
}

func (l *loginRateLimiter) RecordFailure(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.increment(now, "ip:"+ip)
	l.increment(now, "pair:"+ip+"\x00"+username)
	if len(l.entries) > 1024 {
		for key, attempt := range l.entries {
			if now.Sub(attempt.windowStart) >= l.window {
				delete(l.entries, key)
			}
		}
	}
}

func (l *loginRateLimiter) RecordSuccess(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, "pair:"+ip+"\x00"+username)
}

func (l *loginRateLimiter) current(now time.Time, key string) loginAttempt {
	attempt := l.entries[key]
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) >= l.window {
		delete(l.entries, key)
		return loginAttempt{}
	}
	return attempt
}

func (l *loginRateLimiter) increment(now time.Time, key string) {
	attempt := l.current(now, key)
	if attempt.windowStart.IsZero() {
		attempt.windowStart = now
	}
	attempt.failures++
	l.entries[key] = attempt
}
