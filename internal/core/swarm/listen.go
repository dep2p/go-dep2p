package swarm

import (
	"fmt"
	"strings"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
)

// Listen 监听指定地址
func (s *Swarm) Listen(addrs ...string) error {
	if s.closed.Load() {
		return ErrSwarmClosed
	}

	if len(addrs) == 0 {
		return fmt.Errorf("no addresses to listen")
	}

	logger.Debug("开始监听地址", "count", len(addrs))

	var errs []error
	succeeded := 0

	for _, addr := range addrs {
		if err := s.listenAddr(addr); err != nil {
			logger.Warn("监听地址失败", "addr", addr, "error", err)
			errs = append(errs, fmt.Errorf("listen %s: %w", addr, err))
		} else {
			succeeded++
			logger.Debug("监听地址成功", "addr", addr)
		}
	}

	if succeeded == 0 {
		logger.Error("所有地址监听失败", "errors", errs)
		return fmt.Errorf("failed to listen on any address: %v", errs)
	}

	logger.Info("监听成功", "succeeded", succeeded, "total", len(addrs))
	// 至少一个监听成功
	return nil
}

// listenAddr 监听单个地址
func (s *Swarm) listenAddr(addr string) error {
	// 解析地址为 Multiaddr
	maddr, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("parse addr %s: %w", addr, err)
	}

	// 选择传输层
	transport := s.selectTransportForListen(addr)
	if transport == nil {
		return fmt.Errorf("%w: %s", ErrNoTransport, addr)
	}

	// 创建监听器
	listener, err := transport.Listen(maddr)
	if err != nil {
		return fmt.Errorf("transport listen: %w", err)
	}

	// 保存监听器
	s.mu.Lock()
	s.listeners = append(s.listeners, listener)
	s.mu.Unlock()

	// 启动 Accept 循环（异步）
	go s.acceptLoop(listener)

	return nil
}

// acceptLoop 接受连接循环
func (s *Swarm) acceptLoop(listener pkgif.Listener) {
	defer func() {
		if err := listener.Close(); err != nil {
			logger.Debug("关闭监听器时出错", "error", err)
		}
	}()

	for {
		// 检查是否已关闭
		if s.closed.Load() {
			return
		}

		// 接受原始连接
		rawConn, err := listener.Accept()
		if err != nil {
			// 如果是关闭错误，退出循环
			if s.closed.Load() {
				return
			}
			// 其他错误继续
			continue
		}

		// 异步处理连接
		go s.acceptConn(rawConn)
	}
}

// acceptConn 处理接受的连接
func (s *Swarm) acceptConn(transportConn pkgif.Connection) {
	// Transport.Accept() 返回的已经是升级后的 Connection
	// 不需要再次升级

	// 获取远程节点 ID
	peerID := string(transportConn.RemotePeer())
	if peerID == "" {
		logger.Debug("接受的连接无 PeerID，关闭")
		transportConn.Close()
		return
	}

	// 检查是否连接到自己
	if peerID == s.localPeer {
		logger.Debug("拒绝连接到自己的连接")
		transportConn.Close()
		return
	}

	// 使用 truncateID 安全截断 PeerID，避免长度不足时 panic
	peerLabel := truncateID(peerID, 8)
	logger.Debug("接受新连接", "peerID", peerLabel)

	// 封装为 Swarm 连接
	conn := newSwarmConn(s, transportConn)

	// 添加到连接池
	s.addConn(conn)

	// 触发事件
	s.notifyConnected(conn)

	// P0-2: 添加连接类型标签，便于 NAT 穿透效果分析
	logger.Info("连接已建立", "peerID", peerLabel, "connType", conn.ConnType().String())

	// 启动入站流处理循环
	go s.handleInboundStreams(conn)
}

// handleInboundStreams 处理入站流循环
//
// 对于每个连接，启动一个 goroutine 循环接受入站流，
// 并将流传递给 Host 层进行协议协商和路由。
// 当连接关闭或出错时，负责清理连接。
func (s *Swarm) handleInboundStreams(conn *SwarmConn) {
	peerID := string(conn.RemotePeer())
	peerLabel := peerID
	if len(peerLabel) > 8 {
		peerLabel = peerLabel[:8]
	}

	logger.Debug("启动入站流处理循环", "peerID", peerLabel)

	// 使用 defer 确保连接关闭时触发清理
	defer func() {
		// 如果连接尚未关闭，说明是远端关闭或网络错误导致的退出
		// 需要主动清理连接
		if !conn.IsClosed() && !s.closed.Load() {
			logger.Debug("检测到连接异常断开，清理连接", "peerID", peerLabel)
			conn.Close()
		}
	}()

	for {
		// 检查连接是否已关闭
		if conn.IsClosed() || s.closed.Load() {
			logger.Debug("连接已关闭，退出入站流处理", "peerID", peerLabel)
			return
		}

		// 接受入站流
		stream, err := conn.AcceptStream()
		if err != nil {
			// 连接关闭或出错，退出循环（defer 会处理清理）
			if conn.IsClosed() || s.closed.Load() {
				return
			}
			logger.Debug("接受入站流失败，连接可能已断开", "peerID", peerLabel, "error", err)
			return
		}

		// 获取入站流处理器
		handler := s.getInboundStreamHandler()
		if handler == nil {
			logger.Warn("入站流处理器未设置，关闭流", "peerID", peerLabel)
			stream.Reset()
			continue
		}

		// 异步处理入站流
		go handler(stream)
	}
}

// ListenAddrs 返回所有监听地址
func (s *Swarm) ListenAddrs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed.Load() {
		return nil
	}

	addrs := make([]string, 0, len(s.listeners))
	for _, listener := range s.listeners {
		maddr := listener.Addr()
		if maddr != nil {
			addrs = append(addrs, maddr.String())
		}
	}
	return addrs
}

// selectTransportForListen 根据地址选择传输层（监听用）
func (s *Swarm) selectTransportForListen(addr string) pkgif.Transport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 解析地址，选择对应的传输层
	if strings.Contains(addr, "/quic") {
		if t, ok := s.transports["quic"]; ok {
			return t
		}
	}

	if strings.Contains(addr, "/tcp") {
		if t, ok := s.transports["tcp"]; ok {
			return t
		}
	}

	// 默认返回第一个可用传输层
	for _, t := range s.transports {
		return t
	}

	return nil
}
