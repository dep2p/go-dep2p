package gateway

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/gateway"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
//                              中继服务
// ============================================================================

// RelayService 中继服务
type RelayService struct {
	mu sync.RWMutex

	// 父网关
	gateway *Gateway

	// 活跃会话
	sessions     map[string]interfaces.RelaySession
	sessionCount atomic.Int64
}

// NewRelayService 创建中继服务
func NewRelayService(gateway *Gateway) *RelayService {
	return &RelayService{
		gateway:  gateway,
		sessions: make(map[string]interfaces.RelaySession),
	}
}

// ============================================================================
//                              处理中继请求
// ============================================================================

// HandleRelayRequest 处理中继请求
//
// 处理流程：
// 1. 读取并解析 RelayRequest protobuf
// 2. 验证请求（RealmID、认证等）
// 3. 创建到目标节点的连接
// 4. 建立会话并执行双向转发
// 5. 发送响应
func (rs *RelayService) HandleRelayRequest(ctx context.Context, stream io.ReadWriteCloser) error {
	// 使用标志追踪 stream 是否已交给 session 管理
	streamHandedOff := false
	defer func() {
		// 只有在 stream 未交给 session 时才关闭
		if !streamHandedOff {
			stream.Close()
		}
	}()

	// 1. 读取请求数据
	buf := make([]byte, 64*1024) // 64KB 缓冲区
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read request: %w", err)
	}

	// 2. 解析 protobuf 请求
	req := &pb.RelayRequest{}
	if err := proto.Unmarshal(buf[:n], req); err != nil {
		rs.sendErrorResponse(stream, "invalid request format")
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	// 3. 验证请求
	if len(req.SourcePeerId) == 0 || len(req.TargetPeerId) == 0 {
		rs.sendErrorResponse(stream, "missing peer IDs")
		return ErrInvalidRequest
	}

	// 4. 验证认证（如果有 auth）
	if rs.gateway != nil && rs.gateway.auth != nil {
		valid, err := rs.gateway.auth.Authenticate(ctx, string(req.SourcePeerId), req.AuthProof)
		if err != nil || !valid {
			rs.sendErrorResponse(stream, "authentication failed")
			return ErrAuthFailed
		}
	}

	// 5. 创建中继会话（带源连接）
	relayReq := &interfaces.RelayRequest{
		SourcePeerID: string(req.SourcePeerId),
		TargetPeerID: string(req.TargetPeerId),
		Protocol:     req.Protocol,
		RealmID:      string(req.RealmId),
	}
	session := rs.NewSessionWithConn(relayReq, stream)
	streamHandedOff = true // stream 已交给 session 管理
	defer rs.RemoveSession(session.ID())

	// 6. 获取到目标节点的连接
	targetConn, err := rs.gateway.connPool.Acquire(ctx, string(req.TargetPeerId))
	if err != nil {
		rs.sendErrorResponse(stream, "cannot reach target")
		return fmt.Errorf("failed to connect to target: %w", err)
	}
	defer rs.gateway.connPool.Release(string(req.TargetPeerId), targetConn)

	// 7. 发送成功响应
	rs.sendSuccessResponse(stream)

	// 8. 执行双向转发
	transferErr := session.(*RelaySession).Transfer(ctx, targetConn)

	// 9. 关闭连接
	stream.Close()
	targetConn.Close()

	return transferErr
}

// sendSuccessResponse 发送成功响应
func (rs *RelayService) sendSuccessResponse(stream io.ReadWriteCloser) {
	resp := &pb.RelayResponse{
		Success: true,
	}
	data, _ := proto.Marshal(resp)
	stream.Write(data)
}

// sendErrorResponse 发送错误响应
func (rs *RelayService) sendErrorResponse(stream io.ReadWriteCloser, errMsg string) {
	resp := &pb.RelayResponse{
		Success: false,
		Error:   errMsg,
	}
	data, _ := proto.Marshal(resp)
	stream.Write(data)
}

// ============================================================================
//                              流转发
// ============================================================================

// ForwardStream 双向流转发
func (rs *RelayService) ForwardStream(src, dst io.ReadWriteCloser) error {
	errCh := make(chan error, 2)

	// 双向并发转发（零拷贝）
	go func() {
		_, err := io.Copy(dst, src)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(src, dst)
		errCh <- err
	}()

	// 任一方向错误即终止
	return <-errCh
}

// ============================================================================
//                              会话管理
// ============================================================================

// NewSession 创建中继会话
func (rs *RelayService) NewSession(req *interfaces.RelayRequest) interfaces.RelaySession {
	session := NewRelaySession(req)

	rs.mu.Lock()
	rs.sessions[session.ID()] = session
	rs.mu.Unlock()

	rs.sessionCount.Add(1)

	return session
}

// NewSessionWithConn 创建带源连接的中继会话
func (rs *RelayService) NewSessionWithConn(req *interfaces.RelayRequest, sourceConn io.ReadWriteCloser) interfaces.RelaySession {
	session := NewRelaySessionWithConn(req, sourceConn)

	rs.mu.Lock()
	rs.sessions[session.ID()] = session
	rs.mu.Unlock()

	rs.sessionCount.Add(1)

	return session
}

// GetActiveSessions 获取活跃会话数
func (rs *RelayService) GetActiveSessions() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return len(rs.sessions)
}

// RemoveSession 移除会话
func (rs *RelayService) RemoveSession(sessionID string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if session, ok := rs.sessions[sessionID]; ok {
		session.Close()
		delete(rs.sessions, sessionID)
	}
}

// CleanupSessions 清理已完成的会话
func (rs *RelayService) CleanupSessions() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	for id, session := range rs.sessions {
		stats := session.GetStats()
		if stats.Duration > 0 {
			// 会话已完成
			session.Close()
			delete(rs.sessions, id)
		}
	}
}

// 确保实现接口
var _ interfaces.RelayService = (*RelayService)(nil)
