// Package protocol 提供内置协议实现
package protocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	protocolif "github.com/dep2p/go-dep2p/pkg/interfaces/protocol"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("protocol")

// ============================================================================
//                              协议 ID
// ============================================================================

// v1.1 变更: 所有系统协议 ID 添加 /dep2p/sys/ 前缀
// 系统协议无需 Realm 验证，由 Protocol Router 区分处理
// 引用 pkg/protocolids 唯一真源
var (
	// PingProtocol Ping 协议 ID
	PingProtocol = protocolids.SysPing

	// IdentifyProtocol 身份识别协议 ID
	IdentifyProtocol = protocolids.SysIdentify

	// IdentifyPushProtocol 身份推送协议 ID
	IdentifyPushProtocol = protocolids.SysIdentifyPush

	// RealmAuthProtocol RealmAuth 协议 ID
	RealmAuthProtocol = protocolids.SysRealmAuth
)

// ============================================================================
//                              Ping 协议
// ============================================================================

// PingService Ping 服务
type PingService struct {
	localID types.NodeID

	// 统计
	sentCount     uint64
	receivedCount uint64
	mu            sync.RWMutex
}

// PingResult Ping 结果
type PingResult struct {
	Success bool
	RTT     time.Duration
	Error   error
}

// NewPingService 创建 Ping 服务
func NewPingService(localID types.NodeID) *PingService {
	return &PingService{
		localID: localID,
	}
}

// ID 返回协议 ID
func (p *PingService) ID() types.ProtocolID {
	return PingProtocol
}

// Handle 处理入站 Ping
func (p *PingService) Handle(stream endpoint.Stream) error {
	defer func() { _ = stream.Close() }()

	// 读取 ping 数据
	buf := make([]byte, 32)
	_, err := io.ReadFull(stream, buf)
	if err != nil {
		return err
	}

	// 回复相同数据
	_, err = stream.Write(buf)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.receivedCount++
	p.mu.Unlock()

	log.Debug("处理 Ping 请求",
		"from", stream.Connection().RemoteID().ShortString())

	return nil
}

// Ping 发送 Ping
func (p *PingService) Ping(_ context.Context, stream endpoint.Stream) PingResult {
	start := time.Now()

	// 生成随机数据
	data := make([]byte, 32)
	for i := range data {
		data[i] = byte(i)
	}

	// 设置超时
	_ = stream.SetDeadline(time.Now().Add(10 * time.Second))
	defer func() { _ = stream.SetDeadline(time.Time{}) }()

	// 发送 ping
	if _, err := stream.Write(data); err != nil {
		return PingResult{Error: err}
	}

	// 读取 pong
	resp := make([]byte, 32)
	if _, err := io.ReadFull(stream, resp); err != nil {
		return PingResult{Error: err}
	}

	// 验证响应
	if !bytes.Equal(data, resp) {
		return PingResult{Error: errors.New("ping response mismatch")}
	}

	rtt := time.Since(start)

	p.mu.Lock()
	p.sentCount++
	p.mu.Unlock()

	log.Debug("Ping 成功",
		"rtt", rtt)

	return PingResult{
		Success: true,
		RTT:     rtt,
	}
}

// Stats 返回统计信息
func (p *PingService) Stats() (sent, received uint64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sentCount, p.receivedCount
}

// Handler 返回处理函数
func (p *PingService) Handler() endpoint.ProtocolHandler {
	return func(stream endpoint.Stream) {
		_ = p.Handle(stream)
	}
}

// 确保实现 Protocol 接口
var _ protocolif.Protocol = (*PingService)(nil)

// ============================================================================
//                              Identify 协议
// ============================================================================

// IdentifyService 身份识别服务
type IdentifyService struct {
	localID    types.NodeID
	publicKey  []byte
	listenAddrs []string
	protocols  []types.ProtocolID
	agentVersion string
	protocolVersion string

	// 缓存其他节点的信息
	peerInfo   map[types.NodeID]*IdentifyInfo
	peerInfoMu sync.RWMutex

	// 注册表（获取支持的协议）
	registry *Registry
}

// IdentifyInfo 身份信息
type IdentifyInfo struct {
	// 节点 ID
	NodeID types.NodeID

	// 公钥
	PublicKey []byte

	// 监听地址
	ListenAddrs []string

	// 观察到的地址（对方看到的我方地址）
	ObservedAddr string

	// 支持的协议
	Protocols []types.ProtocolID

	// Agent 版本
	AgentVersion string

	// 协议版本
	ProtocolVersion string

	// 更新时间
	UpdatedAt time.Time
}

// NewIdentifyService 创建身份识别服务
func NewIdentifyService(localID types.NodeID, publicKey []byte, registry *Registry) *IdentifyService {
	return &IdentifyService{
		localID:         localID,
		publicKey:       publicKey,
		agentVersion:    "dep2p/1.0.0",
		protocolVersion: "1.0.0",
		registry:        registry,
		peerInfo:        make(map[types.NodeID]*IdentifyInfo),
	}
}

// ID 返回协议 ID
func (is *IdentifyService) ID() types.ProtocolID {
	return IdentifyProtocol
}

// Handle 处理入站 Identify 请求
func (is *IdentifyService) Handle(stream endpoint.Stream) error {
	defer func() { _ = stream.Close() }()

	remotePeer := stream.Connection().RemoteID()

	log.Debug("处理 Identify 请求",
		"from", remotePeer.ShortString())

	// 发送本地信息
	if err := is.sendIdentify(stream); err != nil {
		return err
	}

	return nil
}

// HandlePush 处理 Identify Push
func (is *IdentifyService) HandlePush(stream endpoint.Stream) error {
	defer func() { _ = stream.Close() }()

	// 读取对方推送的信息
	info, err := is.readIdentify(stream)
	if err != nil {
		return err
	}

	// 缓存信息
	is.peerInfoMu.Lock()
	is.peerInfo[info.NodeID] = info
	is.peerInfoMu.Unlock()

	log.Debug("收到 Identify Push",
		"from", info.NodeID.ShortString(),
		"protocols", len(info.Protocols))

	return nil
}

// Identify 请求对方身份
func (is *IdentifyService) Identify(_ context.Context, stream endpoint.Stream) (*IdentifyInfo, error) {
	_ = stream.SetDeadline(time.Now().Add(30 * time.Second))
	defer func() { _ = stream.SetDeadline(time.Time{}) }()

	// 读取对方信息
	info, err := is.readIdentify(stream)
	if err != nil {
		return nil, err
	}

	// 缓存
	is.peerInfoMu.Lock()
	is.peerInfo[info.NodeID] = info
	is.peerInfoMu.Unlock()

	log.Debug("Identify 成功",
		"peer", info.NodeID.ShortString(),
		"agent", info.AgentVersion)

	return info, nil
}

// Push 推送本地身份信息
func (is *IdentifyService) Push(_ context.Context, stream endpoint.Stream) error {
	_ = stream.SetDeadline(time.Now().Add(30 * time.Second))
	defer func() { _ = stream.SetDeadline(time.Time{}) }()

	return is.sendIdentify(stream)
}

// sendIdentify 发送身份信息
func (is *IdentifyService) sendIdentify(w io.Writer) error {
	var buf bytes.Buffer

	// 获取协议列表
	var protocols []types.ProtocolID
	if is.registry != nil {
		protocols = is.registry.IDs()
	}
	is.protocols = protocols

	// 编码消息
	// 格式: [NodeID][PublicKeyLen][PublicKey][AddrsCount][Addrs...][ProtocolsCount][Protocols...][AgentVersion][ProtocolVersion]

	// NodeID (32 bytes)
	_, _ = buf.Write(is.localID[:])

	// PublicKey (长度受协议限制，安全转换)
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(is.publicKey))) // #nosec G115 -- bounded by key size
	_, _ = buf.Write(is.publicKey)

	// ListenAddrs (数量和长度受协议限制)
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(is.listenAddrs))) // #nosec G115 -- bounded by protocol
	for _, addr := range is.listenAddrs {
		_ = binary.Write(&buf, binary.BigEndian, uint16(len(addr))) // #nosec G115 -- bounded by protocol
		_, _ = buf.WriteString(addr)
	}

	// Protocols (数量和长度受协议限制)
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(protocols))) // #nosec G115 -- bounded by protocol
	for _, proto := range protocols {
		protoStr := string(proto)
		_ = binary.Write(&buf, binary.BigEndian, uint16(len(protoStr))) // #nosec G115 -- bounded by protocol
		_, _ = buf.WriteString(protoStr)
	}

	// AgentVersion (长度受协议限制)
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(is.agentVersion))) // #nosec G115 -- bounded by protocol
	_, _ = buf.WriteString(is.agentVersion)

	// ProtocolVersion (长度受协议限制)
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(is.protocolVersion))) // #nosec G115 -- bounded by protocol
	_, _ = buf.WriteString(is.protocolVersion)

	// 写入长度前缀 + 数据（安全转换：buf.Len() 受协议限制）
	bufLen := buf.Len()
	if bufLen > 1024*1024 { // 1MB limit
		return errors.New("identify message too large")
	}
	length := uint32(bufLen) // #nosec G115 -- bounded by 1MB limit check
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return err
	}

	_, err := w.Write(buf.Bytes())
	return err
}

// readIdentify 读取身份信息
func (is *IdentifyService) readIdentify(r io.Reader) (*IdentifyInfo, error) {
	// 读取长度
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	if length > 1024*1024 { // 1MB limit
		return nil, errors.New("identify message too large")
	}

	// 读取数据
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	buf := bytes.NewReader(data)
	info := &IdentifyInfo{
		UpdatedAt: time.Now(),
	}

	// NodeID
	if _, err := buf.Read(info.NodeID[:]); err != nil {
		return nil, err
	}

	// PublicKey
	var pkLen uint16
	if err := binary.Read(buf, binary.BigEndian, &pkLen); err != nil {
		return nil, err
	}
	info.PublicKey = make([]byte, pkLen)
	if _, err := buf.Read(info.PublicKey); err != nil {
		return nil, err
	}

	// ListenAddrs
	var addrCount uint16
	if err := binary.Read(buf, binary.BigEndian, &addrCount); err != nil {
		return nil, err
	}
	info.ListenAddrs = make([]string, addrCount)
	for i := uint16(0); i < addrCount; i++ {
		var addrLen uint16
		if err := binary.Read(buf, binary.BigEndian, &addrLen); err != nil {
			return nil, err
		}
		addrBytes := make([]byte, addrLen)
		if _, err := buf.Read(addrBytes); err != nil {
			return nil, err
		}
		info.ListenAddrs[i] = string(addrBytes)
	}

	// Protocols
	var protoCount uint16
	if err := binary.Read(buf, binary.BigEndian, &protoCount); err != nil {
		return nil, err
	}
	info.Protocols = make([]types.ProtocolID, protoCount)
	for i := uint16(0); i < protoCount; i++ {
		var protoLen uint16
		if err := binary.Read(buf, binary.BigEndian, &protoLen); err != nil {
			return nil, err
		}
		protoBytes := make([]byte, protoLen)
		if _, err := buf.Read(protoBytes); err != nil {
			return nil, err
		}
		info.Protocols[i] = types.ProtocolID(protoBytes)
	}

	// AgentVersion
	var agentLen uint16
	if err := binary.Read(buf, binary.BigEndian, &agentLen); err != nil {
		return nil, err
	}
	agentBytes := make([]byte, agentLen)
	if _, err := buf.Read(agentBytes); err != nil {
		return nil, err
	}
	info.AgentVersion = string(agentBytes)

	// ProtocolVersion
	var versionLen uint16
	if err := binary.Read(buf, binary.BigEndian, &versionLen); err != nil {
		return nil, err
	}
	versionBytes := make([]byte, versionLen)
	if _, err := buf.Read(versionBytes); err != nil {
		return nil, err
	}
	info.ProtocolVersion = string(versionBytes)

	return info, nil
}

// GetPeerInfo 获取缓存的节点信息
func (is *IdentifyService) GetPeerInfo(peer types.NodeID) (*IdentifyInfo, bool) {
	is.peerInfoMu.RLock()
	defer is.peerInfoMu.RUnlock()

	info, ok := is.peerInfo[peer]
	return info, ok
}

// SetListenAddrs 设置监听地址
func (is *IdentifyService) SetListenAddrs(addrs []string) {
	is.listenAddrs = addrs
}

// SetAgentVersion 设置 Agent 版本
func (is *IdentifyService) SetAgentVersion(version string) {
	is.agentVersion = version
}

// Handler 返回处理函数
func (is *IdentifyService) Handler() endpoint.ProtocolHandler {
	return func(stream endpoint.Stream) {
		_ = is.Handle(stream)
	}
}

// PushHandler 返回 Push 处理函数
func (is *IdentifyService) PushHandler() endpoint.ProtocolHandler {
	return func(stream endpoint.Stream) {
		_ = is.HandlePush(stream)
	}
}

// 确保实现 Protocol 接口
var _ protocolif.Protocol = (*IdentifyService)(nil)

// ============================================================================
//                              Echo 协议（用于测试）
// ============================================================================

// EchoProtocol Echo 协议 ID
// 引用 pkg/protocolids 唯一真源
var EchoProtocol = protocolids.SysEcho

// EchoService Echo 服务
type EchoService struct {
}

// NewEchoService 创建 Echo 服务
func NewEchoService() *EchoService {
	return &EchoService{}
}

// ID 返回协议 ID
func (e *EchoService) ID() types.ProtocolID {
	return EchoProtocol
}

// Handle 处理 Echo 请求
func (e *EchoService) Handle(stream endpoint.Stream) error {
	defer func() { _ = stream.Close() }()

	buf := make([]byte, 1024)
	for {
		n, err := stream.Read(buf)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if _, err := stream.Write(buf[:n]); err != nil {
			return err
		}
	}
}

// Handler 返回处理函数
func (e *EchoService) Handler() endpoint.ProtocolHandler {
	return func(stream endpoint.Stream) {
		_ = e.Handle(stream)
	}
}

// 确保实现 Protocol 接口
var _ protocolif.Protocol = (*EchoService)(nil)

// ============================================================================
//                              RegisterBuiltinProtocols
// ============================================================================

// RegisterBuiltinProtocols 注册所有内置协议
func RegisterBuiltinProtocols(registry *Registry, router *Router, localID types.NodeID, publicKey []byte) {
	// Ping 协议
	pingService := NewPingService(localID)
	_ = registry.RegisterWithHandler(PingProtocol, pingService.Handler(),
		WithDescription("Ping/Pong latency measurement"),
		WithPriority(100))
	router.AddHandler(PingProtocol, pingService.Handler())

	// Identify 协议
	identifyService := NewIdentifyService(localID, publicKey, registry)
	_ = registry.RegisterWithHandler(IdentifyProtocol, identifyService.Handler(),
		WithDescription("Peer identification and capability exchange"),
		WithPriority(100))
	router.AddHandler(IdentifyProtocol, identifyService.Handler())

	// Identify Push 协议
	_ = registry.RegisterWithHandler(IdentifyPushProtocol, identifyService.PushHandler(),
		WithDescription("Push identity updates to peers"),
		WithPriority(90))
	router.AddHandler(IdentifyPushProtocol, identifyService.PushHandler())

	// Echo 协议（测试用）
	echoService := NewEchoService()
	_ = registry.RegisterWithHandler(EchoProtocol, echoService.Handler(),
		WithDescription("Echo service for testing"),
		WithPriority(50))
	router.AddHandler(EchoProtocol, echoService.Handler())

	log.Info("已注册内置协议",
		"count", 4)
}

// ============================================================================
//                              BuiltinProtocolsInfo
// ============================================================================

// BuiltinProtocolInfo 内置协议信息
type BuiltinProtocolInfo struct {
	ID          types.ProtocolID
	Name        string
	Description string
}

// GetBuiltinProtocols 获取内置协议列表
func GetBuiltinProtocols() []BuiltinProtocolInfo {
	return []BuiltinProtocolInfo{
		{
			ID:          PingProtocol,
			Name:        "Ping",
			Description: "Ping/Pong latency measurement protocol",
		},
		{
			ID:          IdentifyProtocol,
			Name:        "Identify",
			Description: "Peer identification and capability exchange",
		},
		{
			ID:          IdentifyPushProtocol,
			Name:        "Identify Push",
			Description: "Push identity updates to connected peers",
		},
		{
			ID:          EchoProtocol,
			Name:        "Echo",
			Description: "Echo service for testing and debugging",
		},
	}
}

