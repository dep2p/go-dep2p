package identify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 协议 ID（使用统一定义）
var (
	// ProtocolID Identify 协议 ID
	ProtocolID = string(protocol.Identify)

	// ProtocolIDPush Identify Push 协议 ID（v1.1+）
	ProtocolIDPush = string(protocol.IdentifyPush)
)

var (
	// ErrPushNotImplemented Push 协议尚未实现
	ErrPushNotImplemented = errors.New("identify push protocol not implemented in v1.0")
)

// IdentifyInfo 节点身份信息
type IdentifyInfo struct {
	// PeerID 节点 ID
	PeerID string `json:"peer_id"`

	// PublicKey 公钥（base64 编码）
	PublicKey string `json:"public_key"`

	// ListenAddrs 监听地址列表
	ListenAddrs []string `json:"listen_addrs"`

	// ObservedAddr 观测到的远端地址
	ObservedAddr string `json:"observed_addr"`

	// Protocols 支持的协议列表
	Protocols []string `json:"protocols"`

	// AgentVersion 代理版本
	AgentVersion string `json:"agent_version"`

	// ProtocolVersion 协议版本
	ProtocolVersion string `json:"protocol_version"`
}

// Service Identify 服务
type Service struct {
	host     pkgif.Host
	registry pkgif.ProtocolRegistry
}

// NewService 创建 Identify 服务
func NewService(host pkgif.Host, registry pkgif.ProtocolRegistry) *Service {
	return &Service{
		host:     host,
		registry: registry,
	}
}

// Handler 处理 Identify 请求（服务器端）
// 返回本节点的身份信息
func (s *Service) Handler(stream pkgif.Stream) {
	defer stream.Close()

	// 防御性检查：host 为 nil 时返回空响应
	if s.host == nil {
		return
	}

	// 构造 Identify 消息
	info := &IdentifyInfo{
		PeerID:          s.host.ID(),
		ListenAddrs:     s.host.Addrs(),
		Protocols:       s.getProtocols(),
		AgentVersion:    "go-dep2p/1.0.0",
		ProtocolVersion: "dep2p/1.0.0",
	}

	// Phase 11 修复：正确填充 PublicKey
	if peerstore := s.host.Peerstore(); peerstore != nil {
		// 获取本地节点 ID
		localPeerID := types.PeerID(s.host.ID())

		// 尝试获取本地节点的公钥
		if pubKey, err := peerstore.PubKey(localPeerID); err == nil && pubKey != nil {
			// 获取原始公钥字节
			if rawKey, err := pubKey.Raw(); err == nil {
				// Base64 编码
				info.PublicKey = base64.StdEncoding.EncodeToString(rawKey)
			}
		}
	}

	// 添加 ObservedAddr（远端看到的我方地址）
	info.ObservedAddr = ObserveAddr(stream)

	// 发送 JSON 编码的消息
	encoder := json.NewEncoder(stream)
	_ = encoder.Encode(info)
}

// Identify 主动识别节点（客户端）
// 返回远端节点的身份信息
func Identify(ctx context.Context, host pkgif.Host, peer string) (*IdentifyInfo, error) {
	// 1. 创建流
	stream, err := host.NewStream(ctx, peer, ProtocolID)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	// 2. 接收 Identify 消息
	info := &IdentifyInfo{}
	decoder := json.NewDecoder(stream)
	err = decoder.Decode(info)
	if err != nil {
		return nil, err
	}

	return info, nil
}

// getProtocols 获取所有注册的协议
func (s *Service) getProtocols() []string {
	if s.registry == nil {
		return []string{}
	}

	protocols := s.registry.Protocols()
	strs := make([]string, len(protocols))
	for i, p := range protocols {
		strs[i] = string(p)
	}
	return strs
}

// ObserveAddr 从流中获取观测地址
//
// 返回连接的远端地址（对方看到的我方地址）。
func ObserveAddr(stream pkgif.Stream) string {
	conn := stream.Conn()
	if conn == nil {
		return ""
	}

	// 获取远端地址
	if remoteAddr := conn.RemoteMultiaddr(); remoteAddr != nil {
		return remoteAddr.String()
	}

	return ""
}

// Push 推送身份更新（v1.1+，暂不实现）
//
// 该功能计划在 v1.1 版本中实现。
// 当前版本调用此方法将返回 ErrPushNotImplemented。
func (s *Service) Push(_ context.Context, _ string) error {
	return ErrPushNotImplemented
}
