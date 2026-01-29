package host

import (
	"context"
	"errors"
	"fmt"

	"github.com/dep2p/go-dep2p/internal/core/peerstore"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// 事件定义
// ============================================================================

// ConnectedEvent 连接建立事件
//
// 当 Swarm 成功建立到远程节点的连接时发布。
type ConnectedEvent struct {
	// RemotePeer 远程节点 ID
	RemotePeer types.PeerID
	// Connection 连接对象
	Connection pkgif.Connection
}

// DisconnectedEvent 连接断开事件
//
// 当 Swarm 断开与远程节点的连接时发布。
type DisconnectedEvent struct {
	// RemotePeer 远程节点 ID
	RemotePeer types.PeerID
	// Connection 连接对象
	Connection pkgif.Connection
}

// Start 启动 Host 和所有子系统
func (h *Host) Start(ctx context.Context) error {
	if !h.started.CompareAndSwap(false, true) {
		return errors.New("host already started")
	}

	if h.closed.Load() {
		return errors.New("host is closed")
	}

	logger.Info("正在启动 Host")

	// 1. 启动 NAT 服务（传入 host 以注册协议）
	if h.nat != nil {
		if err := h.nat.Start(ctx, h); err != nil {
			logger.Error("启动 NAT 服务失败", "error", err)
			return fmt.Errorf("failed to start NAT service: %w", err)
		}
		logger.Debug("NAT 服务已启动")
	}

	// 2. 启动 Relay 管理器
	if h.relay != nil {
		if err := h.relay.Start(ctx); err != nil {
			logger.Error("启动 Relay 管理器失败", "error", err)
			return fmt.Errorf("failed to start relay manager: %w", err)
		}
		logger.Debug("Relay 管理器已启动")
	}

	// 3. 启动地址管理器
	if h.addrsManager != nil {
		if err := h.addrsManager.Start(); err != nil {
			// 地址管理器启动失败不阻塞 Host 启动
			logger.Warn("地址管理器启动失败", "error", err)
		} else {
			logger.Debug("地址管理器已启动")
		}
	}

	logger.Info("Host 启动成功")
	return nil
}

// Connected 当 Swarm 建立新连接时调用（实现 SwarmNotifier 接口）
//
// 连接去重：只在首次连接时发布事件，避免同一 peer 的多个连接触发重复事件。
//
// P0 修复：连接建立时将远端地址写入 Peerstore，打通地址传播链路
func (h *Host) Connected(conn pkgif.Connection) {
	if h.closed.Load() {
		return
	}

	peerID := string(conn.RemotePeer())
	peerIDShort := peerID
	if len(peerIDShort) > 8 {
		peerIDShort = peerIDShort[:8]
	}

	// P0 修复：将连接的远端地址写入 Peerstore
	// 这是打通地址传播链路的关键一步：连接建立 → Peerstore 入库 → DHT/Connector 可用
	if h.peerstore != nil {
		if remoteAddr := conn.RemoteMultiaddr(); remoteAddr != nil {
			h.peerstore.AddAddrs(
				types.PeerID(peerID),
				[]types.Multiaddr{remoteAddr},
				peerstore.ConnectedAddrTTL,
			)
			logger.Debug("已将连接地址写入 Peerstore",
				"peerID", peerIDShort,
				"addr", remoteAddr.String())
		}
	}

	// 连接计数去重
	h.peerConnCountMu.Lock()
	h.peerConnCount[peerID]++
	connCount := h.peerConnCount[peerID]
	h.peerConnCountMu.Unlock()

	// 只在首次连接时发布事件
	if connCount > 1 {
		logger.Debug("节点已连接，跳过重复事件", "peerID", peerIDShort, "connCount", connCount)
		return
	}

	logger.Debug("节点连接", "peerID", peerIDShort)

	// 发布连接建立事件到 EventBus
	if h.eventbus != nil {
		// 1. 发布内部事件
		if emitter, err := h.eventbus.Emitter(&ConnectedEvent{}); err == nil {
			emitter.Emit(&ConnectedEvent{
				RemotePeer: conn.RemotePeer(),
				Connection: conn,
			})
			emitter.Close()
		}

		// 2. 发布公开事件（用户可订阅 types.EvtPeerConnected）
		if emitter, err := h.eventbus.Emitter(&types.EvtPeerConnected{}); err == nil {
			stat := conn.Stat()
			direction := types.Direction(stat.Direction)

			emitter.Emit(&types.EvtPeerConnected{
				BaseEvent: types.NewBaseEvent(types.EventTypePeerConnected),
				PeerID:    conn.RemotePeer(),
				Direction: direction,
				NumConns:  connCount,
			})
			emitter.Close()

			// 调试日志：确认 EvtPeerConnected 事件已发送
			logger.Debug("EvtPeerConnected 事件已发送到 EventBus",
				"peerID", peerIDShort,
				"direction", direction,
				"numConns", connCount)
		} else {
			logger.Warn("创建 EvtPeerConnected Emitter 失败", "error", err)
		}
	} else {
		logger.Warn("EventBus 为 nil，无法发送连接事件", "peerID", peerIDShort)
	}

	// 通知 ConnManager（通过标签机制）
	if h.connmgr != nil {
		h.connmgr.TagPeer(peerID, "connection", 10)
	}
}

// DisconnectReasonProvider 断开原因提供者接口
//
// 连接可以实现此接口来提供断开原因，用于快速断开检测机制。
// 参考：design/03_architecture/L3_behavioral/disconnect_detection.md
type DisconnectReasonProvider interface {
	// DisconnectReason 返回断开原因
	DisconnectReason() types.DisconnectReason
	// DisconnectError 返回断开错误（如果有）
	DisconnectError() error
}

// Disconnected 当 Swarm 断开连接时调用（实现 SwarmNotifier 接口）
//
// 连接去重：只在最后一个连接断开时发布事件，避免重复断开事件。
//
// 快速断开检测增强：事件包含断开原因（Reason），用于区分：
// - Graceful: 对端主动关闭
// - Timeout: QUIC 空闲超时
// - Error: 连接错误
// - Local: 本地主动关闭
func (h *Host) Disconnected(conn pkgif.Connection) {
	if h.closed.Load() {
		return
	}

	peerID := string(conn.RemotePeer())
	peerIDShort := peerID
	if len(peerIDShort) > 8 {
		peerIDShort = peerIDShort[:8]
	}

	// 连接计数去重
	h.peerConnCountMu.Lock()
	if h.peerConnCount[peerID] > 0 {
		h.peerConnCount[peerID]--
	}
	connCount := h.peerConnCount[peerID]
	// 清理零计数条目防止内存泄漏
	if connCount == 0 {
		delete(h.peerConnCount, peerID)
	}
	h.peerConnCountMu.Unlock()

	// 只在最后一个连接断开时发布事件
	if connCount > 0 {
		logger.Debug("节点仍有连接，跳过断开事件", "peerID", peerIDShort, "remaining", connCount)
		return
	}

	// 获取断开原因（如果连接实现了 DisconnectReasonProvider 接口）
	reason := types.DisconnectReasonUnknown
	var disconnectErr error
	if provider, ok := conn.(DisconnectReasonProvider); ok {
		reason = provider.DisconnectReason()
		disconnectErr = provider.DisconnectError()
	}

	logger.Debug("节点断开", "peerID", peerIDShort, "reason", reason.String())

	// 发布连接断开事件到 EventBus
	if h.eventbus != nil {
		// 1. 发布内部事件
		if emitter, err := h.eventbus.Emitter(&DisconnectedEvent{}); err == nil {
			emitter.Emit(&DisconnectedEvent{
				RemotePeer: conn.RemotePeer(),
				Connection: conn,
			})
			emitter.Close()
		}

		// 2. 发布公开事件（用户可订阅 types.EvtPeerDisconnected）
		// 快速断开检测：包含断开原因，便于上层判断断开类型
		if emitter, err := h.eventbus.Emitter(&types.EvtPeerDisconnected{}); err == nil {
			emitter.Emit(&types.EvtPeerDisconnected{
				BaseEvent: types.NewBaseEvent(types.EventTypePeerDisconnected),
				PeerID:    conn.RemotePeer(),
				NumConns:  connCount,
				Reason:    reason,
				Error:     disconnectErr,
			})
			emitter.Close()
		}
	}

	// 通知 ConnManager（移除连接标签）
	if h.connmgr != nil {
		h.connmgr.UntagPeer(peerID, "connection")
	}
}

// Started 返回 Host 是否已启动
func (h *Host) Started() bool {
	return h.started.Load()
}
