package resourcemgr

import (
	"sync"
	"sync/atomic"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// streamScope 流作用域
type streamScope struct {
	*resourceScope

	dir   pkgif.Direction  // 流方向
	peer  *peerScope       // 关联的节点作用域
	svc   *serviceScope    // 关联的服务作用域
	proto *protocolScope   // 关联的协议作用域
	rcmgr *resourceManager // 资源管理器

	done   sync.Once   // 确保 Done() 只执行一次
	closed atomic.Bool // 关闭状态
}

// ProtocolScope 返回与此流关联的协议作用域
func (ss *streamScope) ProtocolScope() pkgif.ProtocolScope {
	return ss.proto
}

// SetProtocol 为之前未协商的流设置协议
func (ss *streamScope) SetProtocol(proto types.ProtocolID) error {
	if ss.closed.Load() {
		return ErrResourceScopeClosed
	}

	if ss.proto != nil {
		// 已经设置过协议
		return nil
	}

	// 获取协议作用域
	protoScope := ss.rcmgr.getProtocolScope(proto)

	// 确定流数
	nstreams := 1
	var nstreamsIn, nstreamsOut int
	switch ss.dir {
	case pkgif.DirInbound:
		nstreamsIn = 1
	case pkgif.DirOutbound:
		nstreamsOut = 1
	}

	// 在协议作用域中预留资源
	if err := protoScope.reserveStreams(nstreams, nstreamsIn, nstreamsOut); err != nil {
		protoScope.DecRef()
		return err
	}

	// 从临时作用域释放资源
	ss.rcmgr.transient.releaseStreams(nstreams, nstreamsIn, nstreamsOut)

	ss.proto = protoScope
	return nil
}

// ServiceScope 返回与此流关联的服务作用域
func (ss *streamScope) ServiceScope() pkgif.ServiceScope {
	return ss.svc
}

// SetService 为流设置服务
func (ss *streamScope) SetService(service string) error {
	if ss.closed.Load() {
		return ErrResourceScopeClosed
	}

	if ss.svc != nil {
		// 已经设置过服务
		return nil
	}

	// 获取服务作用域
	svcScope := ss.rcmgr.getServiceScope(service)

	// 确定流数
	nstreams := 1
	var nstreamsIn, nstreamsOut int
	switch ss.dir {
	case pkgif.DirInbound:
		nstreamsIn = 1
	case pkgif.DirOutbound:
		nstreamsOut = 1
	}

	// 在服务作用域中预留资源
	if err := svcScope.reserveStreams(nstreams, nstreamsIn, nstreamsOut); err != nil {
		svcScope.DecRef()
		return err
	}

	// 如果已设置协议，从临时作用域释放资源
	if ss.proto != nil {
		ss.rcmgr.transient.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
	}

	ss.svc = svcScope
	return nil
}

// PeerScope 返回与此流关联的节点作用域
func (ss *streamScope) PeerScope() pkgif.PeerScope {
	return ss.peer
}

// ReserveMemory 预留内存
func (ss *streamScope) ReserveMemory(size int, prio uint8) error {
	if ss.closed.Load() {
		return ErrResourceScopeClosed
	}

	if size < 0 {
		return nil
	}

	// 检查自己的限制
	if err := ss.reserveMemory(size, prio); err != nil {
		return err
	}

	// 预留成功，增加计数
	ss.addMemory(size)
	return nil
}

// ReleaseMemory 释放内存
func (ss *streamScope) ReleaseMemory(size int) {
	if size <= 0 {
		return
	}
	ss.releaseMemory(size)
}

// Done 结束流作用域并释放所有资源
func (ss *streamScope) Done() {
	ss.done.Do(func() {
		ss.closed.Store(true)

		// 确定流数
		nstreams := 1
		var nstreamsIn, nstreamsOut int
		switch ss.dir {
		case pkgif.DirInbound:
			nstreamsIn = 1
		case pkgif.DirOutbound:
			nstreamsOut = 1
		}

		// 释放自己的资源
		ss.releaseStreams(nstreams, nstreamsIn, nstreamsOut)

		// 释放系统作用域资源
		ss.rcmgr.system.releaseStreams(nstreams, nstreamsIn, nstreamsOut)

		// 释放节点作用域资源
		if ss.peer != nil {
			ss.peer.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
			ss.peer.DecRef()
		}

		// 释放协议或临时作用域资源
		if ss.proto != nil {
			ss.proto.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
			ss.proto.DecRef()
		} else if ss.svc == nil {
			// 如果既没有协议也没有服务，释放临时作用域资源
			ss.rcmgr.transient.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
		}

		// 释放服务作用域资源
		if ss.svc != nil {
			ss.svc.releaseStreams(nstreams, nstreamsIn, nstreamsOut)
			ss.svc.DecRef()
		}
	})
}
