package utils

import (
	"sync"
	"time"
)

// ==========================
// 内存级幂等锁表（单实例）
// ==========================

// lockEntry 是内部使用的锁信息
type lockEntry struct {
	expireAt time.Time
}

// IdempotentLockTable 管理所有内存级锁
type IdempotentLockTable struct {
	locks sync.Map
	ttl   time.Duration
	stop  chan struct{}
}

// DefaultLockTTL 默认锁过期时间（180 秒）
const DefaultLockTTL = 180 * time.Second

// globalLockTable 是全局默认实例
var globalLockTable = NewIdempotentLockTable(DefaultLockTTL)

// NewIdempotentLockTable 创建一个锁表，并启动清理协程
func NewIdempotentLockTable(ttl time.Duration) *IdempotentLockTable {
	t := &IdempotentLockTable{
		ttl:  ttl,
		stop: make(chan struct{}),
	}

	// 后台清理过期锁（每20秒清理一次）
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				t.locks.Range(func(k, v any) bool {
					entry := v.(lockEntry)
					if now.After(entry.expireAt) {
						t.locks.Delete(k)
					}
					return true
				})
			case <-t.stop:
				return
			}
		}
	}()
	return t
}

// Stop 清理协程（一般用于测试或优雅退出）
func (t *IdempotentLockTable) Stop() {
	close(t.stop)
}

// TryLock 尝试获取指定 key 的锁
// 返回 true 表示加锁成功，false 表示锁已存在且未过期
func (t *IdempotentLockTable) TryLock(key string) bool {
	now := time.Now()
	_, loaded := t.locks.LoadOrStore(key, lockEntry{expireAt: now.Add(t.ttl)})
	return !loaded
}

// Unlock 释放锁（提前释放，不等过期）
func (t *IdempotentLockTable) Unlock(key string) {
	t.locks.Delete(key)
}

// ==========================
// 便捷全局方法
// ==========================

// IdempotentLock 尝试加锁（使用全局锁表）
func IdempotentLock(key string) bool {
	return globalLockTable.TryLock(key)
}

// IdempotentUnlock 释放锁
func IdempotentUnlock(key string) {
	globalLockTable.Unlock(key)
}
