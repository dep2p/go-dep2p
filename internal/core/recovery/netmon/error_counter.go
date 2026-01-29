// Package netmon 提供网络状态监控功能
package netmon

import (
	"strings"
	"sync"
	"time"
)

// ============================================================================
//                              错误计数器
// ============================================================================

// ErrorCounter 错误计数器
//
// 跟踪每个节点的错误，用于判断节点是否不可达。
type ErrorCounter struct {
	mu sync.RWMutex

	// 配置
	config *Config

	// 每个节点的错误记录
	peerErrors map[string]*peerErrorRecord

	// 最后一次关键错误
	lastCriticalError     error
	lastCriticalErrorPeer string
	lastCriticalErrorTime time.Time
}

// peerErrorRecord 单个节点的错误记录
type peerErrorRecord struct {
	// 错误时间戳列表（用于滑动窗口）
	errorTimes []time.Time

	// 连续错误计数
	consecutiveErrors int

	// 最后一次错误
	lastError error

	// 最后一次错误时间
	lastErrorTime time.Time

	// 最后一次成功时间
	lastSuccessTime time.Time
}

// NewErrorCounter 创建错误计数器
func NewErrorCounter(config *Config) *ErrorCounter {
	return &ErrorCounter{
		config:     config,
		peerErrors: make(map[string]*peerErrorRecord),
	}
}

// RecordError 记录错误
//
// 返回：
// - reachedThreshold: 是否达到错误阈值
// - isCritical: 是否是关键错误
func (c *ErrorCounter) RecordError(peer string, err error) (reachedThreshold bool, isCritical bool) {
	if err == nil {
		return false, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// 获取或创建节点记录
	record, ok := c.peerErrors[peer]
	if !ok {
		record = &peerErrorRecord{}
		c.peerErrors[peer] = record
	}

	// 更新错误记录
	record.consecutiveErrors++
	record.lastError = err
	record.lastErrorTime = now
	record.errorTimes = append(record.errorTimes, now)

	// 清理过期的错误记录
	c.cleanExpiredErrors(record)

	// 检查是否是关键错误
	isCritical = c.isCriticalError(err)
	if isCritical {
		c.lastCriticalError = err
		c.lastCriticalErrorPeer = peer
		c.lastCriticalErrorTime = now
	}

	// 检查是否达到阈值
	reachedThreshold = record.consecutiveErrors >= c.config.ErrorThreshold

	return reachedThreshold, isCritical
}

// RecordSuccess 记录成功
//
// 重置节点的连续错误计数。
func (c *ErrorCounter) RecordSuccess(peer string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	record, ok := c.peerErrors[peer]
	if !ok {
		record = &peerErrorRecord{}
		c.peerErrors[peer] = record
	}

	record.consecutiveErrors = 0
	record.lastSuccessTime = time.Now()
}

// GetFailingPeers 获取失败的节点列表
//
// 返回连续错误次数达到阈值的节点。
func (c *ErrorCounter) GetFailingPeers() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var failing []string
	for peer, record := range c.peerErrors {
		if record.consecutiveErrors >= c.config.ErrorThreshold {
			failing = append(failing, peer)
		}
	}
	return failing
}

// GetHealthyPeers 获取健康的节点列表
//
// 返回连续错误次数未达到阈值的节点。
func (c *ErrorCounter) GetHealthyPeers() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var healthy []string
	for peer, record := range c.peerErrors {
		if record.consecutiveErrors < c.config.ErrorThreshold {
			healthy = append(healthy, peer)
		}
	}
	return healthy
}

// GetPeerErrorCount 获取指定节点的错误计数
func (c *ErrorCounter) GetPeerErrorCount(peer string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	record, ok := c.peerErrors[peer]
	if !ok {
		return 0
	}
	return record.consecutiveErrors
}

// GetLastCriticalError 获取最后一次关键错误
func (c *ErrorCounter) GetLastCriticalError() (peer string, t time.Time, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastCriticalErrorPeer, c.lastCriticalErrorTime, c.lastCriticalError
}

// Reset 重置所有计数
func (c *ErrorCounter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.peerErrors = make(map[string]*peerErrorRecord)
	c.lastCriticalError = nil
	c.lastCriticalErrorPeer = ""
	c.lastCriticalErrorTime = time.Time{}
}

// ResetPeer 重置指定节点的计数
func (c *ErrorCounter) ResetPeer(peer string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.peerErrors, peer)
}

// cleanExpiredErrors 清理过期的错误记录
func (c *ErrorCounter) cleanExpiredErrors(record *peerErrorRecord) {
	if c.config.ErrorWindow <= 0 {
		return
	}

	cutoff := time.Now().Add(-c.config.ErrorWindow)
	var validTimes []time.Time
	for _, t := range record.errorTimes {
		if t.After(cutoff) {
			validTimes = append(validTimes, t)
		}
	}
	record.errorTimes = validTimes
}

// isCriticalError 检查是否是关键错误
func (c *ErrorCounter) isCriticalError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	for _, critical := range c.config.CriticalErrors {
		if strings.Contains(errStr, strings.ToLower(critical)) {
			return true
		}
	}
	return false
}

// TotalPeerCount 返回跟踪的节点总数
func (c *ErrorCounter) TotalPeerCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.peerErrors)
}
