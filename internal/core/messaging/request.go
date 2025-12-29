// Package messaging 提供消息服务模块的实现
package messaging

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              请求响应模式
// ============================================================================

// Request 发送请求并等待响应
func (s *MessagingService) Request(ctx context.Context, nodeID types.NodeID, protocol types.ProtocolID, data []byte) ([]byte, error) {
	if atomic.LoadInt32(&s.closed) == 1 {
		return nil, ErrServiceClosed
	}

	if s.endpoint == nil {
		return nil, ErrNoConnection
	}

	// 获取连接
	conn, ok := s.endpoint.Connection(nodeID)
	if !ok {
		// 尝试连接
		var err error
		conn, err = s.endpoint.Connect(ctx, nodeID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNoConnection, err)
		}
	}

	// 打开流
	stream, err := conn.OpenStream(ctx, ProtocolRequest)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStreamFailed, err)
	}
	defer func() { _ = stream.Close() }()

	// 生成请求 ID
	reqID := atomic.AddUint64(&s.requestID, 1)

	// 构建请求
	req := &types.Request{
		ID:       reqID,
		Protocol: protocol,
		Data:     data,
		From:     s.endpoint.ID(),
	}

	// 发送请求
	if err := writeRequest(stream, req); err != nil {
		return nil, err
	}

	// 读取响应
	resp, err := readResponse(stream)
	if err != nil {
		return nil, err
	}

	if !resp.IsSuccess() {
		return nil, fmt.Errorf("request failed: %s", resp.Error)
	}

	return resp.Data, nil
}

// Send 发送通知，不等待响应
func (s *MessagingService) Send(ctx context.Context, nodeID types.NodeID, protocol types.ProtocolID, data []byte) error {
	if atomic.LoadInt32(&s.closed) == 1 {
		return ErrServiceClosed
	}

	if s.endpoint == nil {
		return ErrNoConnection
	}

	// 获取连接
	conn, ok := s.endpoint.Connection(nodeID)
	if !ok {
		var err error
		conn, err = s.endpoint.Connect(ctx, nodeID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrNoConnection, err)
		}
	}

	// 打开流
	stream, err := conn.OpenStream(ctx, ProtocolNotify)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStreamFailed, err)
	}
	defer func() { _ = stream.Close() }()

	// 写入协议 ID
	if err := writeString(stream, string(protocol)); err != nil {
		return err
	}

	// 写入数据
	if err := writeBytes(stream, data); err != nil {
		return err
	}

	return nil
}

// SetRequestHandler 设置请求处理器
func (s *MessagingService) SetRequestHandler(protocol types.ProtocolID, handler messagingif.RequestHandler) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.requestHandlers[protocol] = handler
}

// SetNotifyHandler 设置通知处理器
func (s *MessagingService) SetNotifyHandler(protocol types.ProtocolID, handler messagingif.NotifyHandler) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.notifyHandlers[protocol] = handler
}

// ============================================================================
//                              流处理器
// ============================================================================

// handleRequestStream 处理请求流
func (s *MessagingService) handleRequestStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	// 读取请求
	req, err := readRequest(stream)
	if err != nil {
		log.Debug("读取请求失败", "err", err)
		return
	}

	// 查找处理器
	s.handlerMu.RLock()
	handler := s.requestHandlers[req.Protocol]
	s.handlerMu.RUnlock()

	var resp *types.Response
	if handler != nil {
		resp = handler(req)
	} else {
		resp = &types.Response{
			Status: messagingif.StatusNotFound,
			Error:  "no handler for protocol",
		}
	}

	// 发送响应
	_ = writeResponse(stream, resp) // 发送失败在流关闭时会处理
}

// handleNotifyStream 处理通知流
func (s *MessagingService) handleNotifyStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	conn := stream.Connection()
	if conn == nil {
		return
	}

	// 读取协议 ID
	protocol, err := readString(stream)
	if err != nil {
		return
	}

	// 读取数据
	data, err := readBytes(stream)
	if err != nil {
		return
	}

	// 查找处理器
	s.handlerMu.RLock()
	handler := s.notifyHandlers[types.ProtocolID(protocol)]
	s.handlerMu.RUnlock()

	if handler != nil {
		handler(data, conn.RemoteID())
	}
}

