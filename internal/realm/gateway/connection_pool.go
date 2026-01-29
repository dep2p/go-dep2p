package gateway

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// ============================================================================
//                              连接池
// ============================================================================

// ConnectionPool 连接池
type ConnectionPool struct {
	mu sync.RWMutex

	// 配置
	host           pkgif.Host
	maxConnPerPeer int
	maxConcurrent  int

	// 连接映射
	connections map[string]*connEntry
	totalConns  atomic.Int64

	// 统计
	acquireCount atomic.Int64
	releaseCount atomic.Int64
}

// connEntry 连接条目
type connEntry struct {
	conn     io.ReadWriteCloser
	lastUsed time.Time
	useCount int
}

// NewConnectionPool 创建连接池
func NewConnectionPool(host pkgif.Host, maxConnPerPeer, maxConcurrent int) *ConnectionPool {
	return &ConnectionPool{
		host:           host,
		maxConnPerPeer: maxConnPerPeer,
		maxConcurrent:  maxConcurrent,
		connections:    make(map[string]*connEntry),
	}
}

// ============================================================================
//                              连接管理
// ============================================================================

// Acquire 获取连接
//
// 连接获取流程：
// 1. 检查连接池容量
// 2. 尝试复用已有连接
// 3. 通过 host.NewStream 创建新连接
func (cp *ConnectionPool) Acquire(ctx context.Context, peerID string) (io.ReadWriteCloser, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// 1. 检查是否达到最大并发
	if int(cp.totalConns.Load()) >= cp.maxConcurrent {
		return nil, ErrPoolExhausted
	}

	// 2. 检查是否有可复用连接
	if entry, ok := cp.connections[peerID]; ok {
		// 验证连接是否有效
		if entry.conn != nil {
			entry.lastUsed = time.Now()
			entry.useCount++
			cp.acquireCount.Add(1)
			return entry.conn, nil
		}
		// 连接无效，从池中移除
		delete(cp.connections, peerID)
	}

	// 3. 创建新连接
	if cp.host == nil {
		return nil, ErrNoHost
	}

	// 使用 Gateway 协议创建流（使用统一定义）
	relayProtocol := string(protocol.GatewayRelay)

	stream, err := cp.host.NewStream(ctx, peerID, relayProtocol)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream to %s: %w", peerID, err)
	}

	// 封装为 ReadWriteCloser
	conn := &streamConn{stream: stream}

	// 添加到连接池
	cp.connections[peerID] = &connEntry{
		conn:     conn,
		lastUsed: time.Now(),
		useCount: 1,
	}
	cp.totalConns.Add(1)
	cp.acquireCount.Add(1)

	return conn, nil
}

// streamConn 将 Stream 封装为 io.ReadWriteCloser
type streamConn struct {
	stream pkgif.Stream
}

func (sc *streamConn) Read(p []byte) (n int, err error) {
	return sc.stream.Read(p)
}

func (sc *streamConn) Write(p []byte) (n int, err error) {
	return sc.stream.Write(p)
}

func (sc *streamConn) Close() error {
	return sc.stream.Close()
}

// Release 释放连接
func (cp *ConnectionPool) Release(peerID string, conn io.ReadWriteCloser) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn == nil {
		return
	}

	// 放回连接池
	cp.connections[peerID] = &connEntry{
		conn:     conn,
		lastUsed: time.Now(),
		useCount: 1,
	}

	cp.releaseCount.Add(1)
}

// Remove 移除连接
func (cp *ConnectionPool) Remove(peerID string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if entry, ok := cp.connections[peerID]; ok {
		if entry.conn != nil {
			entry.conn.Close()
		}
		delete(cp.connections, peerID)
		cp.totalConns.Add(-1)
	}
}

// ============================================================================
//                              清理
// ============================================================================

// CleanupIdle 清理空闲连接
func (cp *ConnectionPool) CleanupIdle() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	now := time.Now()
	idleTimeout := 5 * time.Minute

	for peerID, entry := range cp.connections {
		if now.Sub(entry.lastUsed) > idleTimeout {
			if entry.conn != nil {
				entry.conn.Close()
			}
			delete(cp.connections, peerID)
			cp.totalConns.Add(-1)
		}
	}
}

// ============================================================================
//                              统计
// ============================================================================

// GetStats 获取连接池统计
func (cp *ConnectionPool) GetStats() *interfaces.PoolStats {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	totalConns := len(cp.connections)
	activeConns := 0
	idleConns := 0

	now := time.Now()
	for _, entry := range cp.connections {
		if now.Sub(entry.lastUsed) < 10*time.Second {
			activeConns++
		} else {
			idleConns++
		}
	}

	acquireCount := cp.acquireCount.Load()
	releaseCount := cp.releaseCount.Load()
	hitRate := 0.0
	if acquireCount > 0 {
		hitRate = float64(releaseCount) / float64(acquireCount)
	}

	return &interfaces.PoolStats{
		TotalConnections:  totalConns,
		ActiveConnections: activeConns,
		IdleConnections:   idleConns,
		AcquireCount:      acquireCount,
		ReleaseCount:      releaseCount,
		HitRate:           hitRate,
	}
}

// ============================================================================
//                              关闭
// ============================================================================

// Close 关闭连接池
func (cp *ConnectionPool) Close() error {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for peerID, entry := range cp.connections {
		if entry.conn != nil {
			entry.conn.Close()
		}
		delete(cp.connections, peerID)
	}

	cp.totalConns.Store(0)

	return nil
}

// 确保实现接口
var _ interfaces.ConnectionPool = (*ConnectionPool)(nil)
