package nat

import (
	"context"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/proto/autonat"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
//                          AutoNAT 服务端
// ============================================================================

const (
	// AutoNATProtocol AutoNAT 协议 ID
	AutoNATProtocol = string(protocol.AutoNAT)
	
	// AutoNATTimeout 拨号超时
	AutoNATTimeout = 15 * time.Second
)

// AutoNATServer AutoNAT 服务端
//
// 实现 AutoNAT 协议的服务端，为其他节点提供可达性检测服务。
// 
// 工作流程：
//  1. 接收 Dial 请求（包含请求方的观测地址）
//  2. 尝试拨号到请求方
//  3. 返回 Dial 结果（成功/失败，响应时间）
type AutoNATServer struct {
	host   pkgif.Host
	swarm  pkgif.Swarm
	config *AutoNATServerConfig
}

// AutoNATServerConfig AutoNAT 服务端配置
type AutoNATServerConfig struct {
	// MaxRequestsPerPeer 每个节点的最大请求数
	MaxRequestsPerPeer int
	
	// RateLimitInterval 速率限制间隔
	RateLimitInterval time.Duration
	
	// DialTimeout 拨号超时
	DialTimeout time.Duration
}

// DefaultAutoNATServerConfig 默认服务端配置
func DefaultAutoNATServerConfig() *AutoNATServerConfig {
	return &AutoNATServerConfig{
		MaxRequestsPerPeer: 10,
		RateLimitInterval:  time.Minute,
		DialTimeout:        AutoNATTimeout,
	}
}

// NewAutoNATServer 创建 AutoNAT 服务端
func NewAutoNATServer(host pkgif.Host, swarm pkgif.Swarm, config *AutoNATServerConfig) *AutoNATServer {
	if config == nil {
		config = DefaultAutoNATServerConfig()
	}
	
	return &AutoNATServer{
		host:   host,
		swarm:  swarm,
		config: config,
	}
}

// HandleStream 处理 AutoNAT 请求
//
// 流程：
//  1. 读取 Dial 请求
//  2. 验证请求合法性
//  3. 尝试拨号到请求方的地址
//  4. 返回 Dial 结果
func (s *AutoNATServer) HandleStream(stream pkgif.Stream) {
	defer stream.Close()

	ctx, cancel := context.WithTimeout(context.Background(), s.config.DialTimeout)
	defer cancel()

	// 1. 读取 Dial 请求
	req, err := s.readRequest(stream)
	if err != nil {
		s.sendErrorResponse(stream, autonat.ResponseStatus_E_BAD_REQUEST, "failed to read request")
		return
	}

	// 2. 验证请求
	if req.Dial == nil || req.Dial.Peer == nil || len(req.Dial.Peer.Addrs) == 0 {
		s.sendErrorResponse(stream, autonat.ResponseStatus_E_BAD_REQUEST, "no addresses to dial")
		return
	}

	// 3. 获取请求方 PeerID
	remotePeer := string(stream.Conn().RemotePeer())

	// 4. 尝试拨号到请求方的地址
	dialResult := s.dialPeer(ctx, remotePeer, req.Dial.Peer.Addrs)

	// 5. 返回结果
	s.sendResponse(stream, dialResult)
}

// dialPeer 尝试拨号到节点
func (s *AutoNATServer) dialPeer(ctx context.Context, peerID string, _ [][]byte) *autonat.DialResponse {
	if s.swarm == nil {
		return &autonat.DialResponse{
			Status:     autonat.ResponseStatus_E_INTERNAL_ERROR,
			StatusText: "swarm not available",
		}
	}

	// 尝试拨号
	start := time.Now()
	_, err := s.swarm.DialPeer(ctx, peerID)
	_ = time.Since(start)

	if err != nil {
		return &autonat.DialResponse{
			Status:     autonat.ResponseStatus_E_DIAL_ERROR,
			StatusText: err.Error(),
		}
	}

	return &autonat.DialResponse{
		Status:     autonat.ResponseStatus_OK,
		StatusText: "dial successful",
		Addr:       nil, // 可选：返回拨号成功的地址
	}
}

// readRequest 读取 AutoNAT 请求
func (s *AutoNATServer) readRequest(stream pkgif.Stream) (*autonat.Message, error) {
	buf := make([]byte, 4096)
	n, err := stream.Read(buf)
	if err != nil {
		return nil, err
	}

	msg := &autonat.Message{}
	if err := proto.Unmarshal(buf[:n], msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// sendResponse 发送响应
func (s *AutoNATServer) sendResponse(stream pkgif.Stream, dialResp *autonat.DialResponse) {
	msg := &autonat.Message{
		Type:         autonat.MessageType_DIAL_RESPONSE,
		DialResponse: dialResp,
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return
	}

	stream.Write(data)
}

// sendErrorResponse 发送错误响应
func (s *AutoNATServer) sendErrorResponse(stream pkgif.Stream, status autonat.ResponseStatus, text string) {
	dialResp := &autonat.DialResponse{
		Status:     status,
		StatusText: text,
	}

	s.sendResponse(stream, dialResp)
}

// RegisterProtocol 注册 AutoNAT 协议
func (s *AutoNATServer) RegisterProtocol() error {
	if s.host == nil {
		return ErrServiceClosed
	}

	s.host.SetStreamHandler(AutoNATProtocol, func(stream pkgif.Stream) {
		s.HandleStream(stream)
	})

	return nil
}
