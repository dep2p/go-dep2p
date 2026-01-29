// Package streams 实现流协议
package streams

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// mockHost 模拟 Host
type mockHost struct {
	id       string
	handlers map[string]interfaces.StreamHandler
	streams  map[string]*mockStream
	mu       sync.RWMutex
}

func newMockHost(id string) *mockHost {
	return &mockHost{
		id:       id,
		handlers: make(map[string]interfaces.StreamHandler),
		streams:  make(map[string]*mockStream),
	}
}

func (m *mockHost) ID() string {
	return m.id
}

func (m *mockHost) Addrs() []string {
	return []string{"/ip4/127.0.0.1/tcp/0"}
}

func (m *mockHost) Listen(_ ...string) error {
	return nil
}

func (m *mockHost) Connect(_ context.Context, _ string, _ []string) error {
	return nil
}

func (m *mockHost) SetStreamHandler(protocolID string, handler interfaces.StreamHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[protocolID] = handler
}

func (m *mockHost) RemoveStreamHandler(protocolID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.handlers, protocolID)
}

func (m *mockHost) NewStream(_ context.Context, peerID string, protocolIDs ...string) (interfaces.Stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(protocolIDs) == 0 {
		return nil, ErrEmptyProtocol
	}

	protocol := protocolIDs[0]
	stream := newMockStream(protocol, peerID, m.id)
	m.streams[protocol] = stream
	return stream, nil
}

func (m *mockHost) Peerstore() interfaces.Peerstore {
	return nil
}

func (m *mockHost) EventBus() interfaces.EventBus {
	return nil
}

func (m *mockHost) Close() error {
	return nil
}

func (m *mockHost) AdvertisedAddrs() []string {
	return m.Addrs()
}

func (m *mockHost) ShareableAddrs() []string {
	return nil
}

func (m *mockHost) HolePunchAddrs() []string {
	return nil
}

func (m *mockHost) SetReachabilityCoordinator(_ interfaces.ReachabilityCoordinator) {
	// no-op for mock
}

func (m *mockHost) Network() interfaces.Swarm {
	return nil
}

func (m *mockHost) HandleInboundStream(_ interfaces.Stream) {
	// Mock implementation: no-op
}

// mockStream 模拟 Stream
type mockStream struct {
	protocol   string
	remotePeer string
	localPeer  string
	buf        []byte
	closed     bool
	openedAt   time.Time
	mu         sync.RWMutex
}

func newMockStream(protocol, remotePeer, localPeer string) *mockStream {
	return &mockStream{
		protocol:   protocol,
		remotePeer: remotePeer,
		localPeer:  localPeer,
		buf:        make([]byte, 0),
		openedAt:   time.Now(),
	}
}

func (m *mockStream) Read(p []byte) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return 0, io.EOF
	}

	if len(m.buf) == 0 {
		return 0, io.EOF
	}

	n := copy(p, m.buf)
	m.buf = m.buf[n:]
	return n, nil
}

func (m *mockStream) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, ErrStreamClosed
	}

	m.buf = append(m.buf, p...)
	return len(p), nil
}

func (m *mockStream) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockStream) Reset() error {
	return m.Close()
}

func (m *mockStream) CloseWrite() error {
	return nil
}

func (m *mockStream) CloseRead() error {
	return nil
}

func (m *mockStream) SetDeadline(_ time.Time) error {
	return nil
}

func (m *mockStream) SetReadDeadline(_ time.Time) error {
	return nil
}

func (m *mockStream) SetWriteDeadline(_ time.Time) error {
	return nil
}

func (m *mockStream) Protocol() string {
	return m.protocol
}

func (m *mockStream) SetProtocol(protocol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.protocol = protocol
}

func (m *mockStream) Conn() interfaces.Connection {
	return &mockConnection{remotePeer: m.remotePeer}
}

func (m *mockStream) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

func (m *mockStream) Stat() types.StreamStat {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return types.StreamStat{
		Direction:    types.DirUnknown,
		Opened:       m.openedAt,
		Protocol:     types.ProtocolID(m.protocol),
		BytesRead:    0,
		BytesWritten: 0,
	}
}

func (m *mockStream) State() types.StreamState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// mockConnection 模拟 Connection
type mockConnection struct {
	remotePeer string
}

func (m *mockConnection) RemotePeer() types.PeerID {
	return types.PeerID(m.remotePeer)
}

func (m *mockConnection) LocalPeer() types.PeerID {
	return types.PeerID("local-peer")
}

func (m *mockConnection) RemoteAddr() string {
	return "/ip4/127.0.0.1/tcp/0"
}

func (m *mockConnection) LocalAddr() string {
	return "/ip4/127.0.0.1/tcp/0"
}

func (m *mockConnection) LocalMultiaddr() types.Multiaddr {
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	return addr
}

func (m *mockConnection) RemoteMultiaddr() types.Multiaddr {
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	return addr
}

func (m *mockConnection) Stat() interfaces.ConnectionStat {
	return interfaces.ConnectionStat{
		Direction: interfaces.DirOutbound,
		Opened:    time.Now().Unix(),
	}
}

func (m *mockConnection) Close() error {
	return nil
}

func (m *mockConnection) IsClosed() bool {
	return false
}

func (m *mockConnection) ConnType() interfaces.ConnectionType {
	return interfaces.ConnectionTypeDirect
}

func (m *mockConnection) OpenStream(_ context.Context) (interfaces.Stream, error) {
	return nil, nil
}

func (m *mockConnection) AcceptStream() (interfaces.Stream, error) {
	return nil, nil
}

func (m *mockConnection) Streams() []interfaces.Stream {
	return nil
}

func (m *mockConnection) GetStreams() []interfaces.Stream {
	return nil
}

func (m *mockConnection) NewStream(_ context.Context) (interfaces.Stream, error) {
	return nil, nil
}

// mockRealmManager 模拟 RealmManager
type mockRealmManager struct {
	realms map[string]*mockRealm
	mu     sync.RWMutex
}

func newMockRealmManager() *mockRealmManager {
	return &mockRealmManager{
		realms: make(map[string]*mockRealm),
	}
}

func (m *mockRealmManager) CreateWithOpts(_ context.Context, opts ...interfaces.RealmOption) (interfaces.Realm, error) {
	cfg := &interfaces.RealmConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	realm := &mockRealm{
		id:      cfg.ID,
		name:    cfg.Name,
		members: make(map[string]bool),
	}
	m.realms[cfg.ID] = realm
	return realm, nil
}

func (m *mockRealmManager) GetRealm(realmID string) (interfaces.Realm, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	realm, ok := m.realms[realmID]
	return realm, ok
}

func (m *mockRealmManager) ListRealms() []interfaces.Realm {
	m.mu.RLock()
	defer m.mu.RUnlock()

	realms := make([]interfaces.Realm, 0, len(m.realms))
	for _, r := range m.realms {
		realms = append(realms, r)
	}
	return realms
}

func (m *mockRealmManager) Current() interfaces.Realm {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 返回第一个 realm（如果有）
	for _, r := range m.realms {
		return r
	}
	return nil
}

func (m *mockRealmManager) NotifyNetworkChange(_ context.Context, _ interfaces.NetworkChangeEvent) error {
	return nil
}

func (m *mockRealmManager) Close() error {
	return nil
}

// mockRealm 模拟 Realm
type mockRealm struct {
	id      string
	name    string
	members map[string]bool
	mu      sync.RWMutex
}

func (m *mockRealm) ID() string {
	return m.id
}

func (m *mockRealm) Name() string {
	return m.name
}

func (m *mockRealm) Join(_ context.Context) error {
	return nil
}

func (m *mockRealm) Leave(ctx context.Context) error {
	return nil
}

func (m *mockRealm) Members() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members := make([]string, 0, len(m.members))
	for id := range m.members {
		members = append(members, id)
	}
	return members
}

func (m *mockRealm) IsMember(peerID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.members[peerID]
}

func (m *mockRealm) AddMember(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.members[peerID] = true
}

func (m *mockRealm) Messaging() interfaces.Messaging {
	return nil
}

func (m *mockRealm) PubSub() interfaces.PubSub {
	return nil
}

func (m *mockRealm) Streams() interfaces.Streams {
	return nil
}

func (m *mockRealm) Liveness() interfaces.Liveness {
	return nil
}

func (m *mockRealm) PSK() []byte {
	return nil
}

func (m *mockRealm) Authenticate(ctx context.Context, peerID string, proof []byte) (bool, error) {
	return true, nil
}

func (m *mockRealm) GenerateProof(ctx context.Context) ([]byte, error) {
	return []byte("mock-proof"), nil
}

func (m *mockRealm) Close() error {
	return nil
}

func (m *mockRealm) EventBus() interfaces.EventBus {
	return nil
}

func (m *mockRealm) Connect(ctx context.Context, target string) (interfaces.Connection, error) {
	return nil, nil
}

func (m *mockRealm) ConnectWithHint(ctx context.Context, target string, hints []string) (interfaces.Connection, error) {
	return nil, nil
}

// ProtocolID 生成协议ID(用于测试)
func (m *mockRealm) ProtocolID(suffix string) string {
	return "/dep2p/app/" + m.id + "/" + suffix + "/1.0.0"
}
