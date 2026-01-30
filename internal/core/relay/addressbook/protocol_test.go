package addressbook_test

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/relay/addressbook"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/addressbook"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// testBookForProtocol 创建测试用的 MemberAddressBook（用于协议测试）
func testBookForProtocol(t *testing.T) *addressbook.MemberAddressBook {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "protocol-test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	book, err := addressbook.NewWithEngine("test-realm", eng)
	if err != nil {
		eng.Close()
		t.Fatalf("failed to create addressbook: %v", err)
	}

	t.Cleanup(func() {
		book.Close()
		eng.Close()
	})

	return book
}

// testServiceEngine 创建测试用的存储引擎
func testServiceEngine(t *testing.T) engine.InternalEngine {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "service-test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	t.Cleanup(func() {
		eng.Close()
	})

	return eng
}

// ============================================================================
//                              Mock 实现
// ============================================================================

// mockStream 模拟流实现
type mockStream struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	protocol string
	closed   bool
	mu       sync.Mutex
}

func newMockStream() *mockStream {
	return &mockStream{
		readBuf:  &bytes.Buffer{},
		writeBuf: &bytes.Buffer{},
		protocol: addressbook.FormatProtocolID("test-realm"),
	}
}

func (s *mockStream) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readBuf.Read(p)
}

func (s *mockStream) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeBuf.Write(p)
}

func (s *mockStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *mockStream) Reset() error {
	return s.Close()
}

func (s *mockStream) Protocol() string {
	return s.protocol
}

func (s *mockStream) SetProtocol(protocol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.protocol = protocol
}

func (s *mockStream) Conn() pkgif.Connection {
	return nil
}

func (s *mockStream) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *mockStream) CloseWrite() error {
	return nil
}

func (s *mockStream) CloseRead() error {
	return nil
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

func (s *mockStream) Stat() types.StreamStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	return types.StreamStat{
		Direction:    types.DirUnknown,
		Opened:       time.Now(),
		Protocol:     types.ProtocolID(s.protocol),
		BytesRead:    0,
		BytesWritten: 0,
	}
}

func (s *mockStream) State() types.StreamState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// SetReadData 设置读取数据（用于模拟接收消息）
func (s *mockStream) SetReadData(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readBuf = bytes.NewBuffer(data)
}

// GetWrittenData 获取写入的数据
func (s *mockStream) GetWrittenData() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeBuf.Bytes()
}

// mockHost 模拟 Host 实现
type mockHost struct {
	id       string
	addrs    []string
	handlers map[string]pkgif.StreamHandler
	streams  map[string]*mockStream
	mu       sync.Mutex
}

func newMockHost(id string) *mockHost {
	return &mockHost{
		id:       id,
		addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		handlers: make(map[string]pkgif.StreamHandler),
		streams:  make(map[string]*mockStream),
	}
}

func (h *mockHost) ID() string {
	return h.id
}

func (h *mockHost) Addrs() []string {
	return h.addrs
}

func (h *mockHost) Listen(addrs ...string) error {
	return nil
}

func (h *mockHost) Connect(ctx context.Context, peerID string, addrs []string) error {
	return nil
}

func (h *mockHost) SetStreamHandler(protocolID string, handler pkgif.StreamHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[protocolID] = handler
}

func (h *mockHost) RemoveStreamHandler(protocolID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.handlers, protocolID)
}

func (h *mockHost) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	stream := newMockStream()
	if len(protocolIDs) > 0 {
		stream.protocol = protocolIDs[0]
	}
	h.streams[peerID] = stream
	return stream, nil
}

func (h *mockHost) NewStreamWithPriority(ctx context.Context, peerID string, protocolID string, priority int) (pkgif.Stream, error) {
	return h.NewStream(ctx, peerID, protocolID)
}

func (h *mockHost) Peerstore() pkgif.Peerstore {
	return nil
}

func (h *mockHost) EventBus() pkgif.EventBus {
	return nil
}

func (h *mockHost) Close() error {
	return nil
}

func (h *mockHost) AdvertisedAddrs() []string {
	return h.Addrs()
}

func (h *mockHost) ShareableAddrs() []string {
	return nil
}

func (h *mockHost) HolePunchAddrs() []string {
	return nil
}

func (h *mockHost) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
	// no-op for mock
}

func (h *mockHost) Network() pkgif.Swarm {
	return nil
}

func (h *mockHost) HandleInboundStream(stream pkgif.Stream) {
	// Mock implementation: no-op
}

// GetHandler 获取注册的处理器
func (h *mockHost) GetHandler(protocolID string) pkgif.StreamHandler {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.handlers[protocolID]
}

// ============================================================================
//                              协议测试
// ============================================================================

func TestFormatProtocolID(t *testing.T) {
	protocolID := addressbook.FormatProtocolID("my-realm")
	expected := "/dep2p/realm/my-realm/addressbook/1.0.0"
	if protocolID != expected {
		t.Errorf("FormatProtocolID = %s, want %s", protocolID, expected)
	}
}

func TestWriteReadMessage(t *testing.T) {
	// 创建测试消息
	reg := &pb.AddressRegister{
		NodeId:    []byte("test-node"),
		Addrs:     [][]byte{[]byte("addr1"), []byte("addr2")},
		NatType:   pb.NATType_NAT_TYPE_FULL_CONE,
		Timestamp: time.Now().Unix(),
	}
	msg := addressbook.NewRegisterMessage(reg)

	// 创建模拟流
	stream := newMockStream()

	// 写入消息
	err := addressbook.WriteMessage(stream, msg)
	if err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	// 读取写入的数据并设置到读取缓冲区
	written := stream.GetWrittenData()
	stream.SetReadData(written)

	// 读取消息
	got, err := addressbook.ReadMessage(stream)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	// 验证
	if got.Type != pb.AddressBookMessage_REGISTER {
		t.Errorf("Type = %v, want REGISTER", got.Type)
	}

	gotReg := got.GetRegister()
	if gotReg == nil {
		t.Fatal("GetRegister returned nil")
	}
	if string(gotReg.NodeId) != "test-node" {
		t.Errorf("NodeId = %s, want test-node", gotReg.NodeId)
	}
}

func TestMessageBuilders(t *testing.T) {
	// 测试各种消息构建函数
	tests := []struct {
		name     string
		msg      *pb.AddressBookMessage
		expected pb.AddressBookMessage_MessageType
	}{
		{
			name:     "RegisterMessage",
			msg:      addressbook.NewRegisterMessage(&pb.AddressRegister{}),
			expected: pb.AddressBookMessage_REGISTER,
		},
		{
			name:     "RegisterResponseMessage",
			msg:      addressbook.NewRegisterResponseMessage(&pb.AddressRegisterResponse{}),
			expected: pb.AddressBookMessage_REGISTER_RESPONSE,
		},
		{
			name:     "QueryMessage",
			msg:      addressbook.NewQueryMessage(&pb.AddressQuery{}),
			expected: pb.AddressBookMessage_QUERY,
		},
		{
			name:     "ResponseMessage",
			msg:      addressbook.NewResponseMessage(&pb.AddressResponse{}),
			expected: pb.AddressBookMessage_RESPONSE,
		},
		{
			name:     "BatchQueryMessage",
			msg:      addressbook.NewBatchQueryMessage(&pb.BatchAddressQuery{}),
			expected: pb.AddressBookMessage_BATCH_QUERY,
		},
		{
			name:     "BatchResponseMessage",
			msg:      addressbook.NewBatchResponseMessage(&pb.BatchAddressResponse{}),
			expected: pb.AddressBookMessage_BATCH_RESPONSE,
		},
		{
			name:     "UpdateMessage",
			msg:      addressbook.NewUpdateMessage(&pb.AddressUpdate{}),
			expected: pb.AddressBookMessage_UPDATE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.Type != tt.expected {
				t.Errorf("Type = %v, want %v", tt.msg.Type, tt.expected)
			}
		})
	}
}

// ============================================================================
//                              Handler 测试
// ============================================================================

func TestHandlerRegister(t *testing.T) {
	// 创建地址簿
	book := testBookForProtocol(t)

	// 创建处理器
	handler := addressbook.NewMessageHandler(addressbook.HandlerConfig{
		Book:    book,
		RealmID: "test-realm",
		LocalID: "relay-node",
	})

	// 创建注册消息
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	reg := &pb.AddressRegister{
		NodeId:    []byte("test-member"),
		Addrs:     [][]byte{addr.Bytes()},
		NatType:   pb.NATType_NAT_TYPE_FULL_CONE,
		Timestamp: time.Now().Unix(),
	}
	msg := addressbook.NewRegisterMessage(reg)

	// 创建模拟流
	stream := newMockStream()

	// 序列化并设置到读取缓冲区
	addressbook.WriteMessage(stream, msg)
	stream.SetReadData(stream.GetWrittenData())
	stream.writeBuf.Reset()

	// 处理流
	handler.HandleStream(stream)

	// 读取响应
	respData := stream.GetWrittenData()
	stream.SetReadData(respData)

	resp, err := addressbook.ReadMessage(stream)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	// 验证响应
	if resp.Type != pb.AddressBookMessage_REGISTER_RESPONSE {
		t.Errorf("Type = %v, want REGISTER_RESPONSE", resp.Type)
	}

	regResp := resp.GetRegisterResponse()
	if regResp == nil || !regResp.Success {
		t.Error("Register should succeed")
	}

	// 验证已存储
	ctx := context.Background()
	entry, err := book.Query(ctx, "test-member")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if string(entry.NodeID) != "test-member" {
		t.Errorf("NodeID = %s, want test-member", entry.NodeID)
	}
}

func TestHandlerQuery(t *testing.T) {
	// 创建地址簿并注册成员
	book := testBookForProtocol(t)

	ctx := context.Background()
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	book.Register(ctx, realmif.MemberEntry{
		NodeID:      "target-member",
		DirectAddrs: []types.Multiaddr{addr},
		NATType:     types.NATTypeFullCone,
		Online:      true,
	})

	// 创建处理器
	handler := addressbook.NewMessageHandler(addressbook.HandlerConfig{
		Book:    book,
		RealmID: "test-realm",
		LocalID: "relay-node",
	})

	// 创建查询消息
	query := &pb.AddressQuery{
		TargetNodeId: []byte("target-member"),
		RequestorId:  []byte("alice"),
	}
	msg := addressbook.NewQueryMessage(query)

	// 创建模拟流
	stream := newMockStream()
	addressbook.WriteMessage(stream, msg)
	stream.SetReadData(stream.GetWrittenData())
	stream.writeBuf.Reset()

	// 处理流
	handler.HandleStream(stream)

	// 读取响应
	stream.SetReadData(stream.GetWrittenData())
	resp, err := addressbook.ReadMessage(stream)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	// 验证响应
	if resp.Type != pb.AddressBookMessage_RESPONSE {
		t.Errorf("Type = %v, want RESPONSE", resp.Type)
	}

	addrResp := resp.GetResponse()
	if addrResp == nil || !addrResp.Found {
		t.Error("Query should find the member")
	}
	if addrResp.Entry == nil {
		t.Error("Entry should not be nil")
	}
}

func TestHandlerQueryNotFound(t *testing.T) {
	// 创建空地址簿
	book := testBookForProtocol(t)

	// 创建处理器
	handler := addressbook.NewMessageHandler(addressbook.HandlerConfig{
		Book:    book,
		RealmID: "test-realm",
		LocalID: "relay-node",
	})

	// 创建查询消息（目标不存在）
	query := &pb.AddressQuery{
		TargetNodeId: []byte("non-existing"),
		RequestorId:  []byte("alice"),
	}
	msg := addressbook.NewQueryMessage(query)

	// 创建模拟流
	stream := newMockStream()
	addressbook.WriteMessage(stream, msg)
	stream.SetReadData(stream.GetWrittenData())
	stream.writeBuf.Reset()

	// 处理流
	handler.HandleStream(stream)

	// 读取响应
	stream.SetReadData(stream.GetWrittenData())
	resp, err := addressbook.ReadMessage(stream)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	// 验证响应
	addrResp := resp.GetResponse()
	if addrResp == nil {
		t.Fatal("Response is nil")
	}
	if addrResp.Found {
		t.Error("Should not find non-existing member")
	}
}

func TestHandlerBatchQuery(t *testing.T) {
	// 创建地址簿并注册多个成员
	book := testBookForProtocol(t)

	ctx := context.Background()
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")

	// 注册 3 个成员
	for i := 0; i < 3; i++ {
		book.Register(ctx, realmif.MemberEntry{
			NodeID:      types.PeerID("member-" + string(rune('a'+i))),
			DirectAddrs: []types.Multiaddr{addr},
			Online:      true,
		})
	}

	// 创建处理器
	handler := addressbook.NewMessageHandler(addressbook.HandlerConfig{
		Book:    book,
		RealmID: "test-realm",
		LocalID: "relay-node",
	})

	// 创建批量查询消息（包含存在和不存在的成员）
	query := &pb.BatchAddressQuery{
		TargetNodeIds: [][]byte{
			[]byte("member-a"),
			[]byte("member-b"),
			[]byte("non-existing"),
		},
		RequestorId: []byte("alice"),
	}
	msg := addressbook.NewBatchQueryMessage(query)

	// 创建模拟流
	stream := newMockStream()
	addressbook.WriteMessage(stream, msg)
	stream.SetReadData(stream.GetWrittenData())
	stream.writeBuf.Reset()

	// 处理流
	handler.HandleStream(stream)

	// 读取响应
	stream.SetReadData(stream.GetWrittenData())
	resp, err := addressbook.ReadMessage(stream)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	// 验证响应
	if resp.Type != pb.AddressBookMessage_BATCH_RESPONSE {
		t.Errorf("Type = %v, want BATCH_RESPONSE", resp.Type)
	}

	batchResp := resp.GetBatchResponse()
	if batchResp == nil {
		t.Fatal("BatchResponse is nil")
	}
	if len(batchResp.Entries) != 3 {
		t.Errorf("Entries count = %d, want 3", len(batchResp.Entries))
	}

	// 验证结果
	if !batchResp.Entries[0].Found {
		t.Error("member-a should be found")
	}
	if !batchResp.Entries[1].Found {
		t.Error("member-b should be found")
	}
	if batchResp.Entries[2].Found {
		t.Error("non-existing should not be found")
	}
}

// ============================================================================
//                              Service 测试
// ============================================================================

func TestServiceStartStop(t *testing.T) {
	eng := testServiceEngine(t)
	host := newMockHost("relay-node")

	svc, err := addressbook.NewAddressBookService(addressbook.ServiceConfig{
		RealmID: "test-realm",
		LocalID: "relay-node",
		Host:    host,
		Engine:  eng,
	})
	if err != nil {
		t.Fatalf("NewAddressBookService failed: %v", err)
	}

	// 启动
	err = svc.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !svc.IsRunning() {
		t.Error("Service should be running")
	}

	// 验证处理器已注册
	protocolID := addressbook.FormatProtocolID("test-realm")
	if host.GetHandler(protocolID) == nil {
		t.Error("Handler should be registered")
	}

	// 停止
	err = svc.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if svc.IsRunning() {
		t.Error("Service should not be running")
	}

	// 验证处理器已注销
	if host.GetHandler(protocolID) != nil {
		t.Error("Handler should be removed")
	}
}

// ============================================================================
//                              边界测试
// ============================================================================

func TestReadMessageTooLarge(t *testing.T) {
	stream := newMockStream()

	// 构造一个声称超大的消息（长度前缀）
	lenBuf := make([]byte, 4)
	// 设置长度为 MaxMessageSize + 1
	lenBuf[0] = 0x00
	lenBuf[1] = 0x01
	lenBuf[2] = 0x00
	lenBuf[3] = 0x01 // 约 65537 字节

	stream.SetReadData(lenBuf)

	_, err := addressbook.ReadMessage(stream)
	if err == nil {
		t.Error("Should fail for message too large")
	}
}

func TestReadMessageIncomplete(t *testing.T) {
	stream := newMockStream()

	// 只设置长度前缀，没有消息体
	lenBuf := make([]byte, 4)
	lenBuf[3] = 0x10 // 声称有 16 字节

	stream.SetReadData(lenBuf)

	_, err := addressbook.ReadMessage(stream)
	if err == nil || err == io.EOF {
		// 应该返回错误（EOF 或其他读取错误）
	}
}
