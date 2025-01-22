// Package peerstore provides types and interfaces for local storage of address information, metadata, and public key material about dep2p peers.
package peerstore

import (
	"context"
	"errors"
	"io"
	"math"
	"time"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/record"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// ErrNotFound 表示在存储中未找到请求的项目
var ErrNotFound = errors.New("item not found")

var (
	// AddressTTL 是地址的过期时间
	// 默认为1小时
	AddressTTL = time.Hour

	// TempAddrTTL 用于短期地址的TTL
	// 默认为2分钟
	TempAddrTTL = time.Minute * 2

	// RecentlyConnectedAddrTTL 用于最近连接过的对等点的地址
	// 表示我们对该对等点的地址有较高的确定性
	// 默认为15分钟
	RecentlyConnectedAddrTTL = time.Minute * 15

	// OwnObservedAddrTTL 用于被其他对等点观察到的我们自己的外部地址
	// 已弃用：观察到的地址将保持到我们与提供它的对等点断开连接为止
	// 默认为30分钟
	OwnObservedAddrTTL = time.Minute * 30
)

// 永久TTL常量
// 使用不同的值以便区分，但实际上都是永久的
const (
	// PermanentAddrTTL 用于"永久地址"（如引导节点）的TTL
	// 值为MaxInt64减去iota
	PermanentAddrTTL = math.MaxInt64 - iota

	// ConnectedAddrTTL 用于直接连接的对等点地址的TTL
	// 这基本上是永久的，因为在断开连接后我们会清除它们并以TempAddrTTL重新添加
	ConnectedAddrTTL
)

// Peerstore 提供线程安全的对等点相关信息存储
type Peerstore interface {
	io.Closer

	AddrBook
	KeyBook
	PeerMetadata
	Metrics
	ProtoBook

	// PeerInfo 返回给定peer.ID的peer.PeerInfo结构
	// 这是Peerstore中关于该对等点信息的一个小切片，对其他服务有用
	PeerInfo(peer.ID) peer.AddrInfo

	// Peers 返回所有内部存储中存储的对等点ID
	Peers() peer.IDSlice

	// RemovePeer 移除除地址外的所有对等点相关信息
	// 要移除地址，请使用`AddrBook.ClearAddrs`或将地址TTL设置为0
	RemovePeer(peer.ID)
}

// PeerMetadata 可以处理任何类型的值
// 值的序列化由实现决定
// 可能不支持动态类型内省，在这种情况下可能需要在序列化器中显式列出类型
//
// 更多信息请参考底层实现的文档
type PeerMetadata interface {
	// Get/Put 是其他对等点相关键值对的简单注册表
	// 如果我们发现经常使用某些内容，它应该成为自己的方法集。这是最后的手段
	Get(p peer.ID, key string) (interface{}, error)
	Put(p peer.ID, key string, val interface{}) error

	// RemovePeer 移除存储的对等点的所有值
	RemovePeer(peer.ID)
}

// AddrBook 保存对等点的多地址
type AddrBook interface {
	// AddAddr 调用 AddAddrs(p, []ma.Multiaddr{addr}, ttl)
	AddAddr(p peer.ID, addr ma.Multiaddr, ttl time.Duration)

	// AddAddrs 为AddrBook提供要使用的地址，具有给定的TTL（生存时间）
	// 在TTL之后，地址将不再有效
	// 如果管理器有更长的TTL，则该地址的操作为空操作
	AddAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration)

	// SetAddr 调用 mgr.SetAddrs(p, addr, ttl)
	SetAddr(p peer.ID, addr ma.Multiaddr, ttl time.Duration)

	// SetAddrs 设置地址的TTL。这会清除之前的任何TTL
	// 当我们收到地址有效性的最佳估计时使用此方法
	SetAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration)

	// UpdateAddrs 更新具有给定oldTTL的给定对等点的关联地址，使其具有给定的newTTL
	UpdateAddrs(p peer.ID, oldTTL time.Duration, newTTL time.Duration)

	// Addrs 返回给定对等点的所有已知（且有效）地址
	Addrs(p peer.ID) []ma.Multiaddr

	// AddrStream 返回一个通道，该通道会接收给定对等点的所有地址
	// 如果在调用后添加了新地址，这些地址也会通过该通道发送
	AddrStream(context.Context, peer.ID) <-chan ma.Multiaddr

	// ClearAddresses 移除所有先前存储的地址
	ClearAddrs(p peer.ID)

	// PeersWithAddrs 返回AddrBook中存储的所有对等点ID
	PeersWithAddrs() peer.IDSlice
}

// CertifiedAddrBook 管理签名的对等点记录和其中包含的"自认证"地址
// 将此接口与`AddrBook`一起使用
//
// 要测试给定的AddrBook/Peerstore实现是否支持认证地址，调用者应使用GetCertifiedAddrBook辅助函数或对CertifiedAddrBook接口进行类型断言：
//
//	if cab, ok := aPeerstore.(CertifiedAddrBook); ok {
//	    cab.ConsumePeerRecord(signedPeerRecord, aTTL)
//	}
type CertifiedAddrBook interface {
	// ConsumePeerRecord 存储签名的对等点记录和其包含的地址，持续时间为ttl
	// 签名对等点记录中包含的地址将在ttl后过期
	// 如果任何地址已存在于对等点存储中，它将在现有ttl和提供的ttl的最大值处过期
	// 当与对等点关联的所有地址（无论是自认证的还是非自认证的）从AddrBook中移除时，签名的对等点记录本身将过期
	//
	// 要删除签名的对等点记录，请使用`AddrBook.UpdateAddrs`、`AddrBook.SetAddrs`或`AddrBook.ClearAddrs`，并将ttl设为0
	// 注意：未来对ConsumePeerRecord的调用不会使先前调用的自认证地址过期
	//
	// `accepted`返回值表示记录已成功处理
	// 如果`accepted`为false但未返回错误，则表示记录被忽略，很可能是因为存在具有更大seq值的同一对等点的较新记录
	//
	// 包含签名对等点记录的信封可以通过调用GetPeerRecord(peerID)获取
	ConsumePeerRecord(s *record.Envelope, ttl time.Duration) (accepted bool, err error)

	// GetPeerRecord 返回包含对等点记录的信封，如果不存在记录则返回nil
	GetPeerRecord(p peer.ID) *record.Envelope
}

// GetCertifiedAddrBook 是一个辅助函数，通过类型断言将AddrBook"向上转换"为CertifiedAddrBook
// 如果给定的AddrBook也是CertifiedAddrBook，它将被返回，ok返回值将为true
// 如果AddrBook不是CertifiedAddrBook，则返回(nil, false)
//
// 注意：由于Peerstore嵌入了AddrBook接口，你也可以调用GetCertifiedAddrBook(myPeerstore)
func GetCertifiedAddrBook(ab AddrBook) (cab CertifiedAddrBook, ok bool) {
	cab, ok = ab.(CertifiedAddrBook)
	return cab, ok
}

// KeyBook 跟踪对等点的密钥
type KeyBook interface {
	// PubKey 返回对等点的公钥
	PubKey(peer.ID) ic.PubKey

	// AddPubKey 存储对等点的公钥
	AddPubKey(peer.ID, ic.PubKey) error

	// PrivKey 返回对等点的私钥（如果已知）
	PrivKey(peer.ID) ic.PrivKey

	// AddPrivKey 存储对等点的私钥
	AddPrivKey(peer.ID, ic.PrivKey) error

	// PeersWithKeys 返回KeyBook中存储的所有对等点ID
	PeersWithKeys() peer.IDSlice

	// RemovePeer 移除与对等点关联的所有密钥
	RemovePeer(peer.ID)
}

// Metrics 跟踪一组对等点的指标
type Metrics interface {
	// RecordLatency 记录新的延迟测量
	RecordLatency(peer.ID, time.Duration)

	// LatencyEWMA 返回对等点延迟所有测量的指数加权移动平均值
	LatencyEWMA(peer.ID) time.Duration

	// RemovePeer 移除存储的对等点的所有指标
	RemovePeer(peer.ID)
}

// ProtoBook 跟踪对等点支持的协议
type ProtoBook interface {
	// GetProtocols 获取对等点支持的协议列表
	GetProtocols(peer.ID) ([]protocol.ID, error)

	// AddProtocols 为对等点添加协议
	AddProtocols(peer.ID, ...protocol.ID) error

	// SetProtocols 设置对等点的协议
	SetProtocols(peer.ID, ...protocol.ID) error

	// RemoveProtocols 移除对等点的指定协议
	RemoveProtocols(peer.ID, ...protocol.ID) error

	// SupportsProtocols 从给定的协议中返回对等点支持的协议集
	// 如果返回的错误不为nil，则结果不确定
	SupportsProtocols(peer.ID, ...protocol.ID) ([]protocol.ID, error)

	// FirstSupportedProtocol 返回对等点在给定协议中支持的第一个协议
	// 如果对等点不支持任何给定的协议，此函数将返回空的protocol.ID和nil错误
	// 如果返回的错误不为nil，则结果不确定
	FirstSupportedProtocol(peer.ID, ...protocol.ID) (protocol.ID, error)

	// RemovePeer 移除与对等点关联的所有协议
	RemovePeer(peer.ID)
}
