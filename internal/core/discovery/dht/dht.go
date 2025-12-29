// Package dht 提供分布式哈希表实现
package dht

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("discovery.dht")

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrKeyNotFound 键未找到
	ErrKeyNotFound = error(errKeyNotFound{})

	// ErrNoNodes 没有可用节点
	ErrNoNodes = error(errNoNodes{})

	// ErrDHTClosed DHT 已关闭
	ErrDHTClosed = error(errDHTClosed{})
)

type errKeyNotFound struct{}

func (errKeyNotFound) Error() string { return "key not found in DHT" }

type errNoNodes struct{}

func (errNoNodes) Error() string { return "no nodes available" }

type errDHTClosed struct{}

func (errDHTClosed) Error() string { return "DHT is closed" }

// ============================================================================
//                              配置
// ============================================================================

// Config DHT 配置
type Config struct {
	// Mode 运行模式
	Mode discoveryif.DHTMode

	// BucketSize K 桶大小
	BucketSize int

	// Alpha 并发查询参数
	Alpha int

	// QueryTimeout 查询超时
	QueryTimeout time.Duration

	// RefreshInterval 刷新间隔
	RefreshInterval time.Duration

	// ReplicationFactor 复制因子
	ReplicationFactor int

	// EnableValueStore 启用值存储
	EnableValueStore bool

	// MaxRecordAge 记录最大存活时间
	MaxRecordAge time.Duration

	// BootstrapPeers 引导节点
	BootstrapPeers []discoveryif.PeerInfo
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Mode:              discoveryif.DHTModeAuto,
		BucketSize:        20,
		Alpha:             3,
		QueryTimeout:      30 * time.Second,
		RefreshInterval:   1 * time.Hour,
		ReplicationFactor: 3,
		EnableValueStore:  true,
		MaxRecordAge:      24 * time.Hour,
		BootstrapPeers:    nil,
	}
}

// ============================================================================
//                              DHT 实现
// ============================================================================

// DHT 分布式哈希表
type DHT struct {
	config Config

	// 本地节点信息
	localID    types.NodeID
	localAddrs []string

	// 当前 Realm
	realmID types.RealmID

	// 路由表
	routingTable *RoutingTable

	// 值存储
	store   map[string]storedValue
	storeMu sync.RWMutex

	// Provider 存储（key -> providers）
	providers   map[string][]providerEntry
	providersMu sync.RWMutex

	// 网络层接口
	network   Network
	networkMu sync.RWMutex

	// 外部地址簿（可选）
	// 用于将发现的 closer_peers/providers 地址写入全局 AddressBook，
	// 作为 Peerstore 类底座，支撑 Endpoint.Connect() 的地址来源。
	addressBook   AddressBookWriter
	addressBookMu sync.RWMutex

	// 身份（用于签名 PeerRecord）
	// T3 修复：使用 IdentityWithPubKey 支持签名验证
	identity   IdentityWithPubKey
	identityMu sync.RWMutex

	// PeerRecord 序列号（单调递增）
	peerRecordSeqno uint64

	// 运行模式
	mode int32

	// 运行状态
	running int32
	ctx     context.Context
	cancel  context.CancelFunc
}

// AddressBookWriter 地址簿写入接口
//
// 用于将 DHT 发现的节点地址写入外部 AddressBook（Peerstore 类底座）。
// 参考：design/protocols/foundation/addressing.md - AddressBook 作为 Peerstore 类底座
type AddressBookWriter interface {
	// Add 添加地址
	Add(nodeID types.NodeID, addrs ...string)
}

// ============================================================================
//                              接口定义
// ============================================================================

// Identity 身份接口（用于签名）
type Identity interface {
	// ID 返回节点 ID
	ID() types.NodeID
	// Sign 签名数据
	Sign(data []byte) ([]byte, error)
}

// IdentityWithPubKey 扩展身份接口（包含公钥）
//
// T3 修复：用于创建可验证的 SignedPeerRecord
type IdentityWithPubKey interface {
	Identity
	// PubKeyBytes 返回公钥字节
	PubKeyBytes() []byte
}

// Network 网络接口
type Network interface {
	// SendFindNode 发送 FIND_NODE 请求
	SendFindNode(ctx context.Context, to types.NodeID, target types.NodeID) ([]discoveryif.PeerInfo, error)

	// SendFindValue 发送 FIND_VALUE 请求
	SendFindValue(ctx context.Context, to types.NodeID, key string) ([]byte, []discoveryif.PeerInfo, error)

	// SendStore 发送 STORE 请求
	//
	// 根据设计文档：{ Key: bytes, Value: bytes, TTL: uint32 }
	SendStore(ctx context.Context, to types.NodeID, key string, value []byte, ttl time.Duration) error

	// SendPing 发送 PING 请求
	SendPing(ctx context.Context, to types.NodeID) (time.Duration, error)

	// SendAddProvider 发送 ADD_PROVIDER 请求（携带 TTL）
	SendAddProvider(ctx context.Context, to types.NodeID, key string, ttl time.Duration) error

	// SendGetProviders 发送 GET_PROVIDERS 请求（返回 providers + closer peers）
	SendGetProviders(ctx context.Context, to types.NodeID, key string) ([]ProviderInfo, []types.NodeID, error)

	// SendRemoveProvider 发送 REMOVE_PROVIDER 请求
	SendRemoveProvider(ctx context.Context, to types.NodeID, key string) error

	// LocalID 返回本地节点 ID
	LocalID() types.NodeID

	// LocalAddrs 返回本地地址
	LocalAddrs() []string

	// UpdateLocalAddrs 更新本地地址
	//
	// T5 修复：当节点地址变化时，同步更新网络适配器的本地地址
	// 确保 DHT 发送请求时携带的 SenderAddrs 是最新的
	UpdateLocalAddrs(addrs []string)
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewDHT 创建 DHT
func NewDHT(config Config, network Network, realmID types.RealmID) *DHT {
	localID := types.NodeID{}
	var localAddrs []string
	if network != nil {
		localID = network.LocalID()
		localAddrs = network.LocalAddrs()
	}

	d := &DHT{
		config:       config,
		localID:      localID,
		localAddrs:   localAddrs,
		realmID:      realmID,
		routingTable: NewRoutingTable(localID, realmID),
		store:        make(map[string]storedValue),
		providers:    make(map[string][]providerEntry),
		network:      network,
		mode:         int32(config.Mode),
		// Layer1 修复：使用当前时间微秒数初始化 seqno，确保重启后单调递增
		peerRecordSeqno: uint64(time.Now().UnixMicro()),
	}

	return d
}

// ============================================================================
//                              接口断言
// ============================================================================

var _ discoveryif.DHT = (*DHT)(nil)
var _ discoveryif.Discoverer = (*DHT)(nil)
var _ discoveryif.AddressUpdater = (*DHT)(nil)
var _ discoveryif.Announcer = (*DHT)(nil)

// ============================================================================
//                              辅助函数
// ============================================================================

// dhtModeString 返回模式的字符串表示
func dhtModeString(m discoveryif.DHTMode) string {
	switch m {
	case discoveryif.DHTModeAuto:
		return "auto"
	case discoveryif.DHTModeServer:
		return "server"
	case discoveryif.DHTModeClient:
		return "client"
	default:
		return "unknown"
	}
}
