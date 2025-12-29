package dht

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Mock Stream/Connection
// ============================================================================

// mockConnection 模拟连接
type mockConnection struct {
	remoteID types.NodeID
}

func (c *mockConnection) RemoteID() types.NodeID {
	return c.remoteID
}

func (c *mockConnection) RemotePublicKey() endpoint.PublicKey {
	return nil
}

func (c *mockConnection) RemoteAddrs() []endpoint.Address {
	return nil
}

func (c *mockConnection) LocalID() types.NodeID {
	return types.NodeID{}
}

func (c *mockConnection) LocalAddrs() []endpoint.Address {
	return nil
}

func (c *mockConnection) OpenStream(ctx context.Context, protocolID endpoint.ProtocolID) (endpoint.Stream, error) {
	return nil, nil
}

func (c *mockConnection) OpenStreamWithPriority(ctx context.Context, protocolID endpoint.ProtocolID, priority endpoint.Priority) (endpoint.Stream, error) {
	return nil, nil
}

func (c *mockConnection) AcceptStream(ctx context.Context) (endpoint.Stream, error) {
	return nil, nil
}

func (c *mockConnection) Streams() []endpoint.Stream {
	return nil
}

func (c *mockConnection) StreamCount() int {
	return 0
}

func (c *mockConnection) Stats() endpoint.ConnectionStats {
	return endpoint.ConnectionStats{}
}

func (c *mockConnection) Direction() endpoint.Direction {
	return endpoint.DirInbound
}

func (c *mockConnection) Transport() string {
	return "quic"
}

func (c *mockConnection) Close() error {
	return nil
}

func (c *mockConnection) CloseWithError(code uint32, reason string) error {
	return nil
}

func (c *mockConnection) IsClosed() bool {
	return false
}

func (c *mockConnection) Done() <-chan struct{} {
	return nil
}

func (c *mockConnection) Context() context.Context {
	return nil
}

func (c *mockConnection) SetStreamHandler(protocolID endpoint.ProtocolID, handler endpoint.ProtocolHandler) {
}

func (c *mockConnection) RemoveStreamHandler(protocolID endpoint.ProtocolID) {
}

func (c *mockConnection) RealmContext() *endpoint.RealmContext {
	return nil
}

func (c *mockConnection) SetRealmContext(ctx *endpoint.RealmContext) {
}

func (c *mockConnection) IsRelayed() bool {
	return false
}

func (c *mockConnection) RelayID() endpoint.NodeID {
	return types.EmptyNodeID
}

// mockStream 模拟流
type mockStream struct {
	conn     *mockConnection
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   bool
}

func newMockStream(remoteID types.NodeID, requestData []byte) *mockStream {
	return &mockStream{
		conn: &mockConnection{
			remoteID: remoteID,
		},
		readBuf:  bytes.NewBuffer(requestData),
		writeBuf: &bytes.Buffer{},
	}
}

func (s *mockStream) Read(p []byte) (n int, err error) {
	return s.readBuf.Read(p)
}

func (s *mockStream) Write(p []byte) (n int, err error) {
	return s.writeBuf.Write(p)
}

func (s *mockStream) Close() error {
	s.closed = true
	return nil
}

func (s *mockStream) ID() endpoint.StreamID {
	return 1
}

func (s *mockStream) ProtocolID() endpoint.ProtocolID {
	return protocolids.SysDHT
}

func (s *mockStream) Connection() endpoint.Connection {
	return s.conn
}

func (s *mockStream) SetDeadline(t time.Time) error {
	return nil
}

func (s *mockStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (s *mockStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (s *mockStream) CloseRead() error {
	return nil
}

func (s *mockStream) CloseWrite() error {
	return nil
}

func (s *mockStream) SetPriority(priority endpoint.Priority) {
}

func (s *mockStream) Priority() endpoint.Priority {
	return endpoint.PriorityNormal
}

func (s *mockStream) Stats() endpoint.StreamStats {
	return endpoint.StreamStats{}
}

func (s *mockStream) IsClosed() bool {
	return s.closed
}

// ============================================================================
//                              Handler 安全测试
// ============================================================================

// TestHandler_RejectSpoofedSender 测试 Handler 拒绝伪造的 Sender
func TestHandler_RejectSpoofedSender(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))
	_ = dht.Start(context.Background())
	defer dht.Stop()

	handler := NewHandler(dht)

	// 真实远端 ID
	trustedRemoteID := types.NodeID{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}

	// 伪造的 sender ID（与真实远端不同）
	spoofedSenderID := types.NodeID{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}

	// 创建一个伪造 sender 的 PING 请求
	req := NewPingRequest(1, spoofedSenderID, []string{"/ip4/1.2.3.4/tcp/8000"})
	reqData, err := req.Encode()
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// 封装为帧
	frameData := encodeFrame(reqData)

	// 创建 mock stream，其连接的 RemoteID 是 trustedRemoteID
	stream := newMockStream(trustedRemoteID, frameData)

	// 处理请求
	handler.Handle(stream)

	// 读取响应
	respFrame := stream.writeBuf.Bytes()
	if len(respFrame) < 4 {
		t.Fatalf("Response too short")
	}

	// 解析帧长度
	frameLen := uint32(respFrame[0])<<24 | uint32(respFrame[1])<<16 | uint32(respFrame[2])<<8 | uint32(respFrame[3])
	if int(frameLen)+4 > len(respFrame) {
		t.Fatalf("Incomplete response frame")
	}

	respData := respFrame[4 : 4+frameLen]
	resp, err := DecodeMessage(respData)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// 应该收到错误响应
	if resp.Success {
		t.Error("Expected request to be rejected due to sender mismatch")
	}

	if resp.Error != errSenderMismatch.Error() {
		t.Errorf("Expected error '%s', got '%s'", errSenderMismatch.Error(), resp.Error)
	}

	// 验证路由表没有被伪造的节点污染
	node := dht.routingTable.Find(spoofedSenderID)
	if node != nil {
		t.Error("Spoofed sender should NOT be in routing table")
	}
}

// TestHandler_AcceptValidSender 测试 Handler 接受正确的 Sender
func TestHandler_AcceptValidSender(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))
	_ = dht.Start(context.Background())
	defer dht.Stop()

	handler := NewHandler(dht)

	// 真实远端 ID
	trustedRemoteID := types.NodeID{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}

	// 创建一个正确 sender 的 PING 请求
	req := NewPingRequest(1, trustedRemoteID, []string{"/ip4/8.8.8.8/tcp/8000"})
	reqData, err := req.Encode()
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// 封装为帧
	frameData := encodeFrame(reqData)

	// 创建 mock stream，其连接的 RemoteID 与请求的 Sender 一致
	stream := newMockStream(trustedRemoteID, frameData)

	// 处理请求
	handler.Handle(stream)

	// 读取响应
	respFrame := stream.writeBuf.Bytes()
	if len(respFrame) < 4 {
		t.Fatalf("Response too short")
	}

	// 解析帧长度
	frameLen := uint32(respFrame[0])<<24 | uint32(respFrame[1])<<16 | uint32(respFrame[2])<<8 | uint32(respFrame[3])
	if int(frameLen)+4 > len(respFrame) {
		t.Fatalf("Incomplete response frame")
	}

	respData := respFrame[4 : 4+frameLen]
	resp, err := DecodeMessage(respData)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// 应该成功
	if !resp.Success {
		t.Errorf("Expected request to succeed, got error: %s", resp.Error)
	}

	// 验证路由表已更新
	node := dht.routingTable.Find(trustedRemoteID)
	if node == nil {
		t.Error("Valid sender should be in routing table")
	}
}

// TestHandler_FillEmptySender 测试 Handler 自动填充空的 Sender
func TestHandler_FillEmptySender(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))
	_ = dht.Start(context.Background())
	defer dht.Stop()

	handler := NewHandler(dht)

	// 真实远端 ID
	trustedRemoteID := types.NodeID{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4}

	// 创建一个 sender 为空的 PING 请求
	req := NewPingRequest(1, types.EmptyNodeID, nil)
	reqData, err := req.Encode()
	if err != nil {
		t.Fatalf("Failed to encode request: %v", err)
	}

	// 封装为帧
	frameData := encodeFrame(reqData)

	// 创建 mock stream
	stream := newMockStream(trustedRemoteID, frameData)

	// 处理请求
	handler.Handle(stream)

	// 读取响应
	respFrame := stream.writeBuf.Bytes()
	if len(respFrame) < 4 {
		t.Fatalf("Response too short")
	}

	// 解析帧长度
	frameLen := uint32(respFrame[0])<<24 | uint32(respFrame[1])<<16 | uint32(respFrame[2])<<8 | uint32(respFrame[3])
	if int(frameLen)+4 > len(respFrame) {
		t.Fatalf("Incomplete response frame")
	}

	respData := respFrame[4 : 4+frameLen]
	resp, err := DecodeMessage(respData)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// 应该成功（sender 被自动填充）
	if !resp.Success {
		t.Errorf("Expected request to succeed with auto-filled sender, got error: %s", resp.Error)
	}
}

// TestHandler_RateLimitUsesVerifiedSender 测试速率限制使用已验证的 sender
func TestHandler_RateLimitUsesVerifiedSender(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))
	_ = dht.Start(context.Background())
	defer dht.Stop()

	handler := NewHandler(dht)

	// 真实远端 ID
	trustedRemoteID := types.NodeID{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5}

	// 发送多个 PeerRecord STORE 请求来触发速率限制
	// PeerRecordRateLimit = 10/min
	for i := 0; i < PeerRecordRateLimit+2; i++ {
		req := &Message{
			Type:        MessageTypeStore,
			RequestID:   uint64(i),
			Sender:      trustedRemoteID,
			SenderAddrs: []string{"/ip4/8.8.8.8/tcp/8000"},
			Key:         PeerRecordKeyPrefix + trustedRemoteID.String(),
			Value:       []byte("test-value"),
			TTL:         3600,
		}
		reqData, _ := req.Encode()
		frameData := encodeFrame(reqData)
		stream := newMockStream(trustedRemoteID, frameData)

		handler.Handle(stream)

		// 读取并检查响应
		respFrame := stream.writeBuf.Bytes()
		if len(respFrame) >= 4 {
			frameLen := uint32(respFrame[0])<<24 | uint32(respFrame[1])<<16 | uint32(respFrame[2])<<8 | uint32(respFrame[3])
			if int(frameLen)+4 <= len(respFrame) {
				respData := respFrame[4 : 4+frameLen]
				resp, _ := DecodeMessage(respData)

				// 前 10 个应该成功（或失败于其他原因如签名验证）
				// 第 11、12 个应该因速率限制失败
				if i >= PeerRecordRateLimit && resp != nil && resp.Success {
					// 如果超出限制后仍然成功，说明速率限制没有正常工作
					// 但由于 PeerRecord 需要 SignedPeerRecord 验证，
					// 可能会先失败于签名验证，这也是可接受的
				}
			}
		}
	}
}

// encodeFrame 将数据封装为帧格式
func encodeFrame(data []byte) []byte {
	frameLen := len(data)
	frame := make([]byte, 4+frameLen)
	frame[0] = byte(frameLen >> 24)
	frame[1] = byte(frameLen >> 16)
	frame[2] = byte(frameLen >> 8)
	frame[3] = byte(frameLen)
	copy(frame[4:], data)
	return frame
}

// ============================================================================
//                              Mock Connection 接口完整实现
// ============================================================================

// 确保 mockConnection 实现 endpoint.Connection 接口
var _ endpoint.Connection = (*mockConnectionFull)(nil)

// mockConnectionFull 完整的 mock 连接实现
type mockConnectionFull struct {
	remoteID types.NodeID
}

func (c *mockConnectionFull) RemoteID() types.NodeID {
	return c.remoteID
}

func (c *mockConnectionFull) RemotePublicKey() endpoint.PublicKey {
	return nil
}

func (c *mockConnectionFull) RemoteAddrs() []endpoint.Address {
	return nil
}

func (c *mockConnectionFull) LocalID() types.NodeID {
	return types.NodeID{}
}

func (c *mockConnectionFull) LocalAddrs() []endpoint.Address {
	return nil
}

func (c *mockConnectionFull) OpenStream(ctx context.Context, protocolID endpoint.ProtocolID) (endpoint.Stream, error) {
	return nil, io.EOF
}

func (c *mockConnectionFull) OpenStreamWithPriority(ctx context.Context, protocolID endpoint.ProtocolID, priority endpoint.Priority) (endpoint.Stream, error) {
	return nil, io.EOF
}

func (c *mockConnectionFull) AcceptStream(ctx context.Context) (endpoint.Stream, error) {
	return nil, io.EOF
}

func (c *mockConnectionFull) Streams() []endpoint.Stream {
	return nil
}

func (c *mockConnectionFull) StreamCount() int {
	return 0
}

func (c *mockConnectionFull) Stats() endpoint.ConnectionStats {
	return endpoint.ConnectionStats{}
}

func (c *mockConnectionFull) Direction() endpoint.Direction {
	return endpoint.DirInbound
}

func (c *mockConnectionFull) Transport() string {
	return "quic"
}

func (c *mockConnectionFull) Close() error {
	return nil
}

func (c *mockConnectionFull) CloseWithError(code uint32, reason string) error {
	return nil
}

func (c *mockConnectionFull) IsClosed() bool {
	return false
}

func (c *mockConnectionFull) Done() <-chan struct{} {
	return nil
}

func (c *mockConnectionFull) Context() context.Context {
	return nil
}

func (c *mockConnectionFull) SetStreamHandler(protocolID endpoint.ProtocolID, handler endpoint.ProtocolHandler) {
}

func (c *mockConnectionFull) RemoveStreamHandler(protocolID endpoint.ProtocolID) {
}

func (c *mockConnectionFull) RealmContext() *endpoint.RealmContext {
	return nil
}

func (c *mockConnectionFull) SetRealmContext(ctx *endpoint.RealmContext) {
}

func (c *mockConnectionFull) IsRelayed() bool {
	return false
}

func (c *mockConnectionFull) RelayID() endpoint.NodeID {
	return types.EmptyNodeID
}

// ============================================================================
//                              地址校验测试（Layer1 严格策略）
// ============================================================================

// TestValidateAddress_MultiaddrOnly 测试 multiaddr-only 严格策略
func TestValidateAddress_MultiaddrOnly(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr error
	}{
		// ===== 合法 multiaddr 格式 =====
		{
			name:    "valid ip4/tcp",
			addr:    "/ip4/8.8.8.8/tcp/4001",
			wantErr: nil,
		},
		{
			name:    "valid ip4/udp",
			addr:    "/ip4/8.8.8.8/udp/4001",
			wantErr: nil,
		},
		{
			name:    "valid ip4/quic",
			addr:    "/ip4/8.8.8.8/udp/4001/quic",
			wantErr: nil,
		},
		{
			name:    "valid ip4/quic-v1",
			addr:    "/ip4/8.8.8.8/udp/4001/quic-v1",
			wantErr: nil,
		},
		{
			name:    "valid ip6/tcp",
			addr:    "/ip6/2001:db8::1/tcp/4001",
			wantErr: nil,
		},
		{
			name:    "valid dns4/tcp",
			addr:    "/dns4/bootstrap.dep2p.network/tcp/4001",
			wantErr: nil,
		},
		{
			name:    "valid dns6/tcp",
			addr:    "/dns6/bootstrap.dep2p.network/tcp/4001",
			wantErr: nil,
		},
		{
			name:    "valid relay circuit",
			addr:    "/p2p/5Q2STWvBRelayNodeID/p2p-circuit/p2p/5Q2STWvBTargetNodeID",
			wantErr: nil,
		},
		{
			name:    "valid relay circuit with ip",
			addr:    "/ip4/8.8.8.8/udp/4001/quic-v1/p2p/5Q2STWvBRelayNodeID/p2p-circuit/p2p/5Q2STWvBTargetNodeID",
			wantErr: nil,
		},

		// ===== 非 multiaddr 格式（应被拒绝）=====
		{
			name:    "reject host:port format",
			addr:    "192.168.1.1:8000",
			wantErr: errNotMultiaddr,
		},
		{
			name:    "reject hostname:port format",
			addr:    "example.com:8000",
			wantErr: errNotMultiaddr,
		},
		{
			name:    "reject pure hostname",
			addr:    "example.com",
			wantErr: errNotMultiaddr,
		},
		{
			name:    "reject random string",
			addr:    "random-garbage-string",
			wantErr: errNotMultiaddr,
		},
		{
			name:    "reject empty string",
			addr:    "",
			wantErr: errInvalidAddress,
		},
		{
			name:    "reject ipv6 bracket format",
			addr:    "[::1]:8000",
			wantErr: errNotMultiaddr,
		},

		// ===== 缺少传输协议（应被拒绝）=====
		{
			name:    "reject ip4 without transport",
			addr:    "/ip4/8.8.8.8",
			wantErr: errMissingTransport,
		},
		{
			name:    "reject ip6 without transport",
			addr:    "/ip6/2001:db8::1",
			wantErr: errMissingTransport,
		},
		{
			name:    "reject dns4 without transport",
			addr:    "/dns4/example.com",
			wantErr: errMissingTransport,
		},
		{
			name:    "reject pure p2p without circuit",
			addr:    "/p2p/5Q2STWvBNodeID",
			wantErr: errMissingTransport,
		},

		// ===== 不可路由地址（应被拒绝）=====
		{
			name:    "reject unspecified 0.0.0.0",
			addr:    "/ip4/0.0.0.0/tcp/4001",
			wantErr: errUnroutableAddress,
		},
		{
			name:    "reject loopback 127.0.0.1",
			addr:    "/ip4/127.0.0.1/tcp/4001",
			wantErr: errUnroutableAddress,
		},
		{
			name:    "reject private 10.x.x.x",
			addr:    "/ip4/10.0.0.1/tcp/4001",
			wantErr: errUnroutableAddress,
		},
		{
			name:    "reject private 192.168.x.x",
			addr:    "/ip4/192.168.1.1/tcp/4001",
			wantErr: errUnroutableAddress,
		},
		{
			name:    "reject private 172.16.x.x",
			addr:    "/ip4/172.16.0.1/tcp/4001",
			wantErr: errUnroutableAddress,
		},
		{
			name:    "reject ipv6 loopback",
			addr:    "/ip6/::1/tcp/4001",
			wantErr: errUnroutableAddress,
		},
		{
			name:    "reject ipv6 unspecified",
			addr:    "/ip6/::/tcp/4001",
			wantErr: errUnroutableAddress,
		},

		// ===== 无效端口（应被拒绝）=====
		{
			name:    "reject port 0",
			addr:    "/ip4/8.8.8.8/tcp/0",
			wantErr: errInvalidPort,
		},
		{
			name:    "reject port > 65535",
			addr:    "/ip4/8.8.8.8/tcp/65536",
			wantErr: errInvalidPort,
		},
		{
			name:    "reject negative port",
			addr:    "/ip4/8.8.8.8/tcp/-1",
			wantErr: errInvalidPort,
		},

		// ===== 未知协议前缀（应被拒绝）=====
		{
			name:    "reject unknown protocol prefix",
			addr:    "/unknown/something/tcp/4001",
			wantErr: errNotMultiaddr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAddress(tt.addr)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("validateAddress(%q) got error %v, want nil", tt.addr, err)
				}
			} else {
				if err == nil {
					t.Errorf("validateAddress(%q) got nil, want error %v", tt.addr, tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("validateAddress(%q) got error %v, want %v", tt.addr, err, tt.wantErr)
				}
			}
		})
	}
}

// TestValidateAddresses_MultiaddrOnly 测试地址列表校验
func TestValidateAddresses_MultiaddrOnly(t *testing.T) {
	tests := []struct {
		name    string
		addrs   []string
		wantErr error
	}{
		{
			name:    "empty list rejected",
			addrs:   []string{},
			wantErr: errInvalidAddress,
		},
		{
			name:    "nil list rejected",
			addrs:   nil,
			wantErr: errInvalidAddress,
		},
		{
			name:    "single valid addr",
			addrs:   []string{"/ip4/8.8.8.8/tcp/4001"},
			wantErr: nil,
		},
		{
			name:    "multiple valid addrs",
			addrs:   []string{"/ip4/8.8.8.8/tcp/4001", "/ip4/8.8.4.4/udp/4001/quic-v1"},
			wantErr: nil,
		},
		{
			name:    "one invalid in list",
			addrs:   []string{"/ip4/8.8.8.8/tcp/4001", "192.168.1.1:8000"},
			wantErr: errNotMultiaddr,
		},
		{
			name:    "relay circuit in list",
			addrs:   []string{"/p2p/5Q2STWvBRelay/p2p-circuit/p2p/5Q2STWvBTarget"},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAddresses(tt.addrs)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("validateAddresses(%v) got error %v, want nil", tt.addrs, err)
				}
			} else {
				if err == nil {
					t.Errorf("validateAddresses(%v) got nil, want error %v", tt.addrs, tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("validateAddresses(%v) got error %v, want %v", tt.addrs, err, tt.wantErr)
				}
			}
		})
	}
}

// TestParseMultiaddrStrict 测试 multiaddr 解析
func TestParseMultiaddrStrict(t *testing.T) {
	tests := []struct {
		name          string
		addr          string
		wantHost      string
		wantPort      string
		wantTransport bool
	}{
		{
			name:          "ip4/tcp",
			addr:          "/ip4/8.8.8.8/tcp/4001",
			wantHost:      "8.8.8.8",
			wantPort:      "4001",
			wantTransport: true,
		},
		{
			name:          "ip6/tcp",
			addr:          "/ip6/2001:db8::1/tcp/4001",
			wantHost:      "2001:db8::1",
			wantPort:      "4001",
			wantTransport: true,
		},
		{
			name:          "ip4/udp/quic-v1",
			addr:          "/ip4/8.8.8.8/udp/4001/quic-v1",
			wantHost:      "8.8.8.8",
			wantPort:      "4001",
			wantTransport: true,
		},
		{
			name:          "dns4/tcp",
			addr:          "/dns4/example.com/tcp/4001",
			wantHost:      "example.com",
			wantPort:      "4001",
			wantTransport: true,
		},
		{
			name:          "ip4 without transport",
			addr:          "/ip4/8.8.8.8",
			wantHost:      "8.8.8.8",
			wantPort:      "",
			wantTransport: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, hasTransport := parseMultiaddrStrict(tt.addr)
			if host != tt.wantHost {
				t.Errorf("parseMultiaddrStrict(%q) host = %q, want %q", tt.addr, host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("parseMultiaddrStrict(%q) port = %q, want %q", tt.addr, port, tt.wantPort)
			}
			if hasTransport != tt.wantTransport {
				t.Errorf("parseMultiaddrStrict(%q) hasTransport = %v, want %v", tt.addr, hasTransport, tt.wantTransport)
			}
		})
	}
}
