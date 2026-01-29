package quic

import (
	"context"
	"net"
	"sync"
	
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/transport/quic")

// RebindSupport Socket 重绑定支持
//
// 当网络接口变化时（如 4G→WiFi），需要关闭旧 socket 并创建新 socket
// 绑定到新的网络接口。
type RebindSupport struct {
	mu          sync.RWMutex
	currentAddr net.Addr
	rebindFunc  func(ctx context.Context) error
}

// NewRebindSupport 创建重绑定支持
func NewRebindSupport() *RebindSupport {
	return &RebindSupport{}
}

// SetRebindFunc 设置重绑定函数
//
// 由传输层实现具体的重绑定逻辑
func (r *RebindSupport) SetRebindFunc(fn func(ctx context.Context) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rebindFunc = fn
}

// Rebind 执行 socket 重绑定
//
// 参考 iroh-main 的实现：
// 1. 关闭旧 socket
// 2. 创建新 socket 绑定到新地址
// 3. 更新本地地址缓存
func (r *RebindSupport) Rebind(ctx context.Context) error {
	r.mu.RLock()
	rebindFunc := r.rebindFunc
	r.mu.RUnlock()

	if rebindFunc == nil {
		logger.Warn("rebind 函数未设置，跳过重绑定")
		return nil
	}

	logger.Debug("执行 socket rebind")
	
	oldAddr := r.GetCurrentAddr()
	
	if err := rebindFunc(ctx); err != nil {
		logger.Warn("socket rebind 失败", "err", err)
		return err
	}

	newAddr := r.GetCurrentAddr()
	
	logger.Info("socket rebind 成功",
		"oldAddr", oldAddr,
		"newAddr", newAddr)
	
	return nil
}

// UpdateAddr 更新当前地址
func (r *RebindSupport) UpdateAddr(addr net.Addr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentAddr = addr
}

// GetCurrentAddr 获取当前地址
func (r *RebindSupport) GetCurrentAddr() net.Addr {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentAddr
}

// TransportRebinder 传输层重绑定接口
type TransportRebinder interface {
	// Rebind 重新绑定 socket 到新网络
	Rebind(ctx context.Context) error
	
	// GetLocalAddr 获取当前本地地址
	GetLocalAddr() net.Addr
}
