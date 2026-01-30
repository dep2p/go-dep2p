package pubsub

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// mockHost 是 Host 的 mock 实现
type mockHost struct {
	id       string
	addrs    []string
	handlers map[string]interfaces.StreamHandler
	mu       sync.RWMutex
}

func newMockHost(id string) *mockHost {
	return &mockHost{
		id:       id,
		addrs:    []string{"/ip4/127.0.0.1/tcp/0"},
		handlers: make(map[string]interfaces.StreamHandler),
	}
}

func (m *mockHost) ID() string {
	return m.id
}

func (m *mockHost) Addrs() []string {
	return m.addrs
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
	return newMockStream(protocolIDs[0], peerID), nil
}

func (m *mockHost) NewStreamWithPriority(ctx context.Context, peerID string, protocolID string, priority int) (interfaces.Stream, error) {
	return m.NewStream(ctx, peerID, protocolID)
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

// mockStream 是 Stream 的 mock 实现
type mockStream struct {
	protocol   string
	remotePeer string
	data       []byte
	mu         sync.Mutex
	closed     bool
}

func newMockStream(protocol, remotePeer string) *mockStream {
	return &mockStream{
		protocol:   protocol,
		remotePeer: remotePeer,
	}
}

func (m *mockStream) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, io.EOF
	}
	if len(m.data) == 0 {
		return 0, io.EOF
	}
	n = copy(p, m.data)
	m.data = m.data[n:]
	return n, nil
}

func (m *mockStream) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, io.ErrClosedPipe
	}
	m.data = append(m.data, p...)
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

func (m *mockStream) Stat() types.StreamStat {
	m.mu.Lock()
	defer m.mu.Unlock()
	return types.StreamStat{
		Direction:    types.DirUnknown,
		Opened:       time.Now(),
		Protocol:     types.ProtocolID(m.protocol),
		BytesRead:    0,
		BytesWritten: 0,
	}
}

func (m *mockStream) State() types.StreamState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

func (m *mockStream) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// mockConnection 是 Connection 的 mock 实现
type mockConnection struct {
	remotePeer string
}

func (m *mockConnection) RemotePeer() types.PeerID {
	return types.PeerID(m.remotePeer)
}

func (m *mockConnection) LocalPeer() types.PeerID {
	return ""
}

func (m *mockConnection) RemoteMultiaddr() types.Multiaddr {
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	return addr
}

func (m *mockConnection) LocalMultiaddr() types.Multiaddr {
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	return addr
}

func (m *mockConnection) NewStream(_ context.Context) (interfaces.Stream, error) {
	return nil, nil
}

func (m *mockConnection) NewStreamWithPriority(_ context.Context, _ int) (interfaces.Stream, error) {
	return nil, nil
}

func (m *mockConnection) SupportsStreamPriority() bool {
	return false
}

func (m *mockConnection) AcceptStream() (interfaces.Stream, error) {
	return nil, nil
}

func (m *mockConnection) GetStreams() []interfaces.Stream {
	return nil
}

func (m *mockConnection) Stat() interfaces.ConnectionStat {
	return interfaces.ConnectionStat{}
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

// mockRealm 是 Realm 的 mock 实现
type mockRealm struct {
	id      string
	name    string
	members map[string]bool
	mu      sync.RWMutex
}

func newMockRealm(id, name string) *mockRealm {
	return &mockRealm{
		id:      id,
		name:    name,
		members: make(map[string]bool),
	}
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

func (m *mockRealm) Leave(_ context.Context) error {
	return nil
}

func (m *mockRealm) Members() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	members := make([]string, 0, len(m.members))
	for peerID := range m.members {
		members = append(members, peerID)
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

func (m *mockRealm) Authenticate(_ context.Context, _ string, _ []byte) (bool, error) {
	return true, nil
}

func (m *mockRealm) GenerateProof(_ context.Context) ([]byte, error) {
	return []byte("mock-proof"), nil
}

func (m *mockRealm) Close() error {
	return nil
}

func (m *mockRealm) EventBus() interfaces.EventBus {
	return nil
}

func (m *mockRealm) Connect(_ context.Context, _ string) (interfaces.Connection, error) {
	return nil, nil
}

func (m *mockRealm) ConnectWithHint(_ context.Context, _ string, _ []string) (interfaces.Connection, error) {
	return nil, nil
}

// mockRealmManager 是 RealmManager 的 mock 实现
type mockRealmManager struct {
	realms map[string]*mockRealm
	mu     sync.RWMutex
}

func newMockRealmManager() *mockRealmManager {
	return &mockRealmManager{
		realms: make(map[string]*mockRealm),
	}
}

func (m *mockRealmManager) CreateWithOpts(ctx context.Context, opts ...interfaces.RealmOption) (interfaces.Realm, error) {
	config := &interfaces.RealmConfig{
		ID:   "test-realm",
		Name: "Test Realm",
	}
	for _, opt := range opts {
		opt(config)
	}

	realm := newMockRealm(config.ID, config.Name)
	m.mu.Lock()
	m.realms[config.ID] = realm
	m.mu.Unlock()

	return realm, nil
}

func (m *mockRealmManager) GetRealm(realmID string) (interfaces.Realm, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	realm, exists := m.realms[realmID]
	return realm, exists
}

func (m *mockRealmManager) ListRealms() []interfaces.Realm {
	m.mu.RLock()
	defer m.mu.RUnlock()

	realms := make([]interfaces.Realm, 0, len(m.realms))
	for _, realm := range m.realms {
		realms = append(realms, realm)
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

func (m *mockRealmManager) NotifyNetworkChange(ctx context.Context, event interfaces.NetworkChangeEvent) error {
	return nil
}

func (m *mockRealmManager) AddRealm(realm *mockRealm) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.realms[realm.ID()] = realm
}

func (m *mockRealmManager) Close() error {
	return nil
}
