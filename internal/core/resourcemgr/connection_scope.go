package resourcemgr

import (
	"sync"
	"sync/atomic"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// connectionScope 连接作用域
type connectionScope struct {
	*resourceScope

	dir      pkgif.Direction   // 连接方向
	usefd    bool              // 是否使用文件描述符
	peer     *peerScope        // 关联的节点作用域
	endpoint types.Multiaddr   // 连接端点地址
	rcmgr    *resourceManager  // 资源管理器

	done   sync.Once   // 确保 Done() 只执行一次
	closed atomic.Bool // 关闭状态
}

// PeerScope 返回与此连接关联的节点作用域
func (cs *connectionScope) PeerScope() pkgif.PeerScope {
	return cs.peer
}

// ReserveMemory 预留内存
func (cs *connectionScope) ReserveMemory(size int, prio uint8) error {
	if cs.closed.Load() {
		return ErrResourceScopeClosed
	}

	if size < 0 {
		return nil
	}

	// 检查自己的限制
	if err := cs.reserveMemory(size, prio); err != nil {
		return err
	}

	// 预留成功，增加计数
	cs.addMemory(size)
	return nil
}

// ReleaseMemory 释放内存
func (cs *connectionScope) ReleaseMemory(size int) {
	if size <= 0 {
		return
	}
	cs.releaseMemory(size)
}

// SetPeer 为之前未关联的连接设置节点
func (cs *connectionScope) SetPeer(peer types.PeerID) error {
	if cs.closed.Load() {
		return ErrResourceScopeClosed
	}

	if cs.peer != nil {
		// 已经设置过节点
		return nil
	}

	// 获取节点作用域
	peerScope := cs.rcmgr.getPeerScope(peer)

	// 确定连接数和 FD 数
	nconns, nfd := 1, 0
	var nconnsIn, nconnsOut int
	switch cs.dir {
	case pkgif.DirInbound:
		nconnsIn = 1
	case pkgif.DirOutbound:
		nconnsOut = 1
	}
	if cs.usefd {
		nfd = 1
	}

	// 在节点作用域中预留资源
	if err := peerScope.reserveConns(nconns, nconnsIn, nconnsOut, nfd); err != nil {
		peerScope.DecRef()
		return err
	}

	// 从临时作用域释放资源
	cs.rcmgr.transient.releaseConns(nconns, nconnsIn, nconnsOut, nfd)

	cs.peer = peerScope
	return nil
}

// ProtectPeer 保护节点，防止被连接管理器修剪
// 注意：本实现中这是一个占位符，实际保护逻辑在 connmgr 中
func (cs *connectionScope) ProtectPeer(_ types.PeerID) {
	// 占位符：实际保护逻辑在 connmgr 中实现
}

// Done 结束连接作用域并释放所有资源
func (cs *connectionScope) Done() {
	cs.done.Do(func() {
		cs.closed.Store(true)

		// 确定连接数和 FD 数
		nconns, nfd := 1, 0
		var nconnsIn, nconnsOut int
		switch cs.dir {
		case pkgif.DirInbound:
			nconnsIn = 1
		case pkgif.DirOutbound:
			nconnsOut = 1
		}
		if cs.usefd {
			nfd = 1
		}

		// 释放自己的资源
		cs.releaseConns(nconns, nconnsIn, nconnsOut, nfd)

		// 释放系统作用域资源
		cs.rcmgr.system.releaseConns(nconns, nconnsIn, nconnsOut, nfd)

		// 释放节点或临时作用域资源
		if cs.peer != nil {
			cs.peer.releaseConns(nconns, nconnsIn, nconnsOut, nfd)
			cs.peer.DecRef()
		} else {
			cs.rcmgr.transient.releaseConns(nconns, nconnsIn, nconnsOut, nfd)
		}
	})
}
