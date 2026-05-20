package utils

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ✅ 限流规则: 每IP 每api qps<MaxRequestsPerSecond
const (
	MaxRequestsPerSecond = 2000
	// 超过此时长未访问的 entry 会被清理
	idleEvictAfter = 10 * time.Minute
	// 清理协程的轮询间隔
	cleanupInterval = 2 * time.Minute
)

type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	ipLimiterMap sync.Map // key -> *ipLimiterEntry
	limiterMu    sync.Mutex
	cleanupOnce  sync.Once
)

// getOrCreateLimiter 返回对应 key 的 limiter，并刷新 lastSeen
func getOrCreateLimiter(key string) *rate.Limiter {
	if v, ok := ipLimiterMap.Load(key); ok {
		entry := v.(*ipLimiterEntry)
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	// double-checked locking: 保证 limiter 唯一
	limiterMu.Lock()
	defer limiterMu.Unlock()
	if v, ok := ipLimiterMap.Load(key); ok {
		entry := v.(*ipLimiterEntry)
		entry.lastSeen = time.Now()
		return entry.limiter
	}
	entry := &ipLimiterEntry{
		// 速率与突发上限均为 MaxRequestsPerSecond * 2，避免瞬时抖动误伤
		limiter:  rate.NewLimiter(rate.Limit(MaxRequestsPerSecond), MaxRequestsPerSecond*2),
		lastSeen: time.Now(),
	}
	ipLimiterMap.Store(key, entry)
	return entry.limiter
}

// startCleanupOnce 后台清理过期 entry，仅启动一次
func startCleanupOnce() {
	cleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(cleanupInterval)
			defer ticker.Stop()
			for range ticker.C {
				cutoff := time.Now().Add(-idleEvictAfter)
				ipLimiterMap.Range(func(k, v any) bool {
					entry := v.(*ipLimiterEntry)
					if entry.lastSeen.Before(cutoff) {
						ipLimiterMap.Delete(k)
					}
					return true
				})
			}
		}()
	})
}

func RateLimitMiddleware() gin.HandlerFunc {
	startCleanupOnce()
	return func(c *gin.Context) {
		ip := c.ClientIP()
		path := c.Request.URL.Path
		if ip == "" {
			ip = "unknown"
		}

		key := ip + ":" + path
		limiter := getOrCreateLimiter(key)
		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": 429,
				"msg":  "rate limit exceeded",
				"data": gin.H{"ip": ip, "path": path},
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
