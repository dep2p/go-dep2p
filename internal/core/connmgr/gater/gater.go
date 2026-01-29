// Package gater 实现连接门控
package gater

import (
	"sync"
)

// Gater 连接门控器
type Gater struct {
	mu sync.RWMutex

	// blockedPeers 黑名单节点
	blockedPeers map[string]struct{}

	// blockedAddrs 黑名单地址
	blockedAddrs map[string]struct{}

	// allowedPeers 白名单节点
	allowedPeers map[string]struct{}
}

// New 创建门控器
func New() *Gater {
	return &Gater{
		blockedPeers: make(map[string]struct{}),
		blockedAddrs: make(map[string]struct{}),
		allowedPeers: make(map[string]struct{}),
	}
}

// InterceptPeerDial 拦截节点拨号
func (g *Gater) InterceptPeerDial(peerID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, blocked := g.blockedPeers[peerID]; blocked {
		return false
	}
	return true
}

// InterceptAddrDial 拦截地址拨号
func (g *Gater) InterceptAddrDial(peerID, addr string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, blocked := g.blockedAddrs[addr]; blocked {
		return false
	}
	return g.InterceptPeerDial(peerID)
}

// InterceptAccept 拦截接受连接
func (g *Gater) InterceptAccept(addr string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, blocked := g.blockedAddrs[addr]; blocked {
		return false
	}
	return true
}

// InterceptSecured 拦截安全连接
func (g *Gater) InterceptSecured(_ int, peerID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, blocked := g.blockedPeers[peerID]; blocked {
		return false
	}
	return true
}

// BlockPeer 添加节点到黑名单
func (g *Gater) BlockPeer(peerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.blockedPeers[peerID] = struct{}{}
}

// UnblockPeer 从黑名单移除节点
func (g *Gater) UnblockPeer(peerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.blockedPeers, peerID)
}

// BlockAddr 添加地址到黑名单
func (g *Gater) BlockAddr(addr string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.blockedAddrs[addr] = struct{}{}
}

// UnblockAddr 从黑名单移除地址
func (g *Gater) UnblockAddr(addr string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.blockedAddrs, addr)
}

// AllowPeer 添加节点到白名单
func (g *Gater) AllowPeer(peerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.allowedPeers[peerID] = struct{}{}
}

// DisallowPeer 从白名单移除节点
func (g *Gater) DisallowPeer(peerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.allowedPeers, peerID)
}
