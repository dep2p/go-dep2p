package types

import (
	"errors"
	"strings"
	"time"
)

// ============================================================================
//                              AddressInfo - 地址信息
// ============================================================================

// AddressInfo 带节点信息的地址
// 用于在发现服务和地址簿之间传递节点地址信息
type AddressInfo struct {
	// ID 节点ID
	ID NodeID

	// Addrs 地址列表（Multiaddr 格式）
	Addrs []Multiaddr
}

// HasAddrs 检查是否有地址
func (ai AddressInfo) HasAddrs() bool {
	return len(ai.Addrs) > 0
}

// AddrsToStrings 返回地址的字符串切片（用于兼容旧 API）
func (ai AddressInfo) AddrsToStrings() []string {
	strs := make([]string, len(ai.Addrs))
	for i, ma := range ai.Addrs {
		strs[i] = ma.String()
	}
	return strs
}

// NewAddressInfo 创建 AddressInfo
func NewAddressInfo(id NodeID, addrs []Multiaddr) AddressInfo {
	return AddressInfo{ID: id, Addrs: addrs}
}

// NewAddressInfoFromStrings 从字符串地址创建 AddressInfo
func NewAddressInfoFromStrings(id NodeID, addrStrs []string) AddressInfo {
	addrs := make([]Multiaddr, 0, len(addrStrs))
	for _, s := range addrStrs {
		ma, err := ParseMultiaddr(s)
		if err == nil {
			addrs = append(addrs, ma)
		}
	}
	return AddressInfo{ID: id, Addrs: addrs}
}

// ============================================================================
//                              AddressRecord - 签名地址记录
// ============================================================================

// AddressRecord 签名的地址记录
// 用于在 DHT 或其他发现服务中发布和验证节点地址
type AddressRecord struct {
	// NodeID 节点ID
	NodeID NodeID

	// RealmID 业务域ID
	RealmID RealmID

	// Seq 序列号（单调递增，用于版本控制）
	Seq uint64

	// Addrs 地址列表（Multiaddr 格式）
	Addrs []Multiaddr

	// Timestamp 记录创建时间
	Timestamp time.Time

	// TTL 记录有效期
	TTL time.Duration

	// Signature 签名（使用私钥对记录内容签名）
	Signature []byte
}

// AddrsToStrings 返回地址的字符串切片（用于兼容旧 API）
func (ar AddressRecord) AddrsToStrings() []string {
	strs := make([]string, len(ar.Addrs))
	for i, ma := range ar.Addrs {
		strs[i] = ma.String()
	}
	return strs
}

// IsExpired 检查记录是否过期
func (ar AddressRecord) IsExpired() bool {
	return time.Since(ar.Timestamp) > ar.TTL
}

// ============================================================================
//                              AddressState - 地址状态
// ============================================================================

// AddressState 地址验证状态
type AddressState int

const (
	// AddressStateUnknown 未知状态
	AddressStateUnknown AddressState = iota
	// AddressStatePending 待验证
	AddressStatePending
	// AddressStateAvailable 可用
	AddressStateAvailable
	// AddressStateInvalid 无效
	AddressStateInvalid
	// AddressStateUnreachable 不可达
	AddressStateUnreachable
)

// String 返回地址状态的字符串表示
func (s AddressState) String() string {
	switch s {
	case AddressStatePending:
		return "pending"
	case AddressStateAvailable:
		return "available"
	case AddressStateInvalid:
		return "invalid"
	case AddressStateUnreachable:
		return "unreachable"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              AddressEntry - 地址簿条目
// ============================================================================

// AddressEntry 地址簿中的单个地址条目
type AddressEntry struct {
	// Addr 地址（Multiaddr 格式）
	Addr Multiaddr

	// State 地址状态
	State AddressState

	// LastSeen 最后一次看到该地址的时间
	LastSeen time.Time

	// LastSuccess 最后一次成功连接的时间
	LastSuccess time.Time

	// FailCount 连续失败次数
	FailCount int

	// Source 地址来源
	Source AddressSource
}

// AddressSource 地址来源
type AddressSource int

const (
	// AddressSourceUnknown 未知来源
	AddressSourceUnknown AddressSource = iota
	// AddressSourceDHT 从 DHT 发现
	AddressSourceDHT
	// AddressSourceMDNS 从 mDNS 发现
	AddressSourceMDNS
	// AddressSourceManual 手动配置
	AddressSourceManual
	// AddressSourceBootstrap 从引导节点获取
	AddressSourceBootstrap
	// AddressSourceRelay 从中继获取
	AddressSourceRelay
	// AddressSourceIncoming 从入站连接获取
	AddressSourceIncoming
)

// String 返回地址来源的字符串表示
func (s AddressSource) String() string {
	switch s {
	case AddressSourceDHT:
		return "dht"
	case AddressSourceMDNS:
		return "mdns"
	case AddressSourceManual:
		return "manual"
	case AddressSourceBootstrap:
		return "bootstrap"
	case AddressSourceRelay:
		return "relay"
	case AddressSourceIncoming:
		return "incoming"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              Relay 地址解析
// ============================================================================

// RelayAddrProtocol 中继地址协议标识
const RelayAddrProtocol = "p2p-circuit"

// RelayAddrInfo 解析后的中继地址信息
type RelayAddrInfo struct {
	// RelayAddr 中继节点的完整地址（包含 /p2p/relayID）
	RelayAddr string

	// RelayID 中继节点 ID
	RelayID NodeID

	// DestID 目标节点 ID（可能为空，表示连接到中继节点自身）
	DestID NodeID
}

// 中继地址解析错误
var (
	// ErrNotRelayAddr 不是中继地址
	ErrNotRelayAddr = errors.New("not a relay address")

	// ErrInvalidRelayAddr 无效的中继地址格式
	ErrInvalidRelayAddr = errors.New("invalid relay address format")

	// ErrMissingRelayID 缺少中继节点 ID
	ErrMissingRelayID = errors.New("missing relay node ID")
)

// IsRelayAddr 检查地址是否为中继地址
//
// 中继地址格式：
//   - /ip4/.../p2p/<relayID>/p2p-circuit
//   - /ip4/.../p2p/<relayID>/p2p-circuit/p2p/<destID>
func IsRelayAddr(addr string) bool {
	return strings.Contains(addr, "/"+RelayAddrProtocol)
}

// ParseRelayAddr 解析中继地址
//
// 输入格式：
//   - /ip4/1.2.3.4/udp/4001/quic-v1/p2p/<relayID>/p2p-circuit
//   - /ip4/1.2.3.4/udp/4001/quic-v1/p2p/<relayID>/p2p-circuit/p2p/<destID>
//
// 返回：
//   - RelayAddrInfo 包含中继地址、中继ID、目标ID
func ParseRelayAddr(addr string) (*RelayAddrInfo, error) {
	if !IsRelayAddr(addr) {
		return nil, ErrNotRelayAddr
	}

	// 按 /p2p-circuit 分割
	parts := strings.Split(addr, "/"+RelayAddrProtocol)
	if len(parts) < 1 || parts[0] == "" {
		return nil, ErrInvalidRelayAddr
	}

	// 解析中继部分：/ip4/.../p2p/<relayID>
	relayPart := parts[0]
	relayID, err := extractNodeIDFromAddr(relayPart)
	if err != nil {
		return nil, ErrMissingRelayID
	}

	info := &RelayAddrInfo{
		RelayAddr: relayPart,
		RelayID:   relayID,
	}

	// 解析目标部分（如果存在）：/p2p/<destID>
	if len(parts) > 1 && parts[1] != "" {
		destPart := parts[1]
		destID, err := extractNodeIDFromAddr(destPart)
		if err == nil {
			info.DestID = destID
		}
	}

	return info, nil
}

// BuildRelayAddr 构建中继地址
//
// 参数：
//   - relayAddr: 中继节点地址（如 /ip4/1.2.3.4/udp/4001/quic-v1/p2p/<relayID>）
//   - destID: 目标节点 ID（可为空）
//
// 返回：
//   - 中继地址字符串
func BuildRelayAddr(relayAddr string, destID NodeID) string {
	result := relayAddr + "/" + RelayAddrProtocol
	if !destID.IsEmpty() {
		result += "/p2p/" + destID.String()
	}
	return result
}

// BuildRelayAddrWithIDs 使用节点 ID 构建中继地址
//
// 参数：
//   - relayBaseAddr: 中继节点基础地址（不含 /p2p/<relayID>）
//   - relayID: 中继节点 ID
//   - destID: 目标节点 ID（可为空）
//
// 返回：
//   - 中继地址字符串
func BuildRelayAddrWithIDs(relayBaseAddr string, relayID NodeID, destID NodeID) string {
	relayAddr := relayBaseAddr + "/p2p/" + relayID.String()
	return BuildRelayAddr(relayAddr, destID)
}

// BuildSimpleRelayCircuit 构建简化的中继电路地址（不含基础地址）
//
// 格式: /p2p/<relayID>/p2p-circuit/p2p/<destID>
//
// 用于内部 relay 连接表示，不包含可拨号的传输地址。
// 外部节点无法直接使用此地址进行拨号，需要结合 relay 节点的基础地址。
func BuildSimpleRelayCircuit(relayID, destID NodeID) Multiaddr {
	if relayID.IsEmpty() {
		return ""
	}
	result := "/p2p/" + relayID.String() + "/" + RelayAddrProtocol
	if !destID.IsEmpty() {
		result += "/p2p/" + destID.String()
	}
	return Multiaddr(result)
}

// extractNodeIDFromAddr 从地址中提取节点 ID
//
// 地址格式：.../p2p/<nodeID>...
func extractNodeIDFromAddr(addr string) (NodeID, error) {
	// 找到 /p2p/ 的位置
	idx := strings.Index(addr, "/p2p/")
	if idx == -1 {
		return NodeID{}, errors.New("no /p2p/ component in address")
	}

	// 提取 nodeID 部分
	remaining := addr[idx+5:] // 跳过 "/p2p/"
	// 找到下一个 / 或者字符串结尾
	endIdx := strings.Index(remaining, "/")
	var nodeIDStr string
	if endIdx == -1 {
		nodeIDStr = remaining
	} else {
		nodeIDStr = remaining[:endIdx]
	}

	if nodeIDStr == "" {
		return NodeID{}, errors.New("empty node ID")
	}

	return ParseNodeID(nodeIDStr)
}

// GetProtocolFromAddr 从地址中提取传输协议
//
// 返回最后一个传输协议标识，如 "quic-v1", "tcp", "p2p-circuit"
func GetProtocolFromAddr(addr string) string {
	// 如果是中继地址，返回 p2p-circuit
	if IsRelayAddr(addr) {
		return RelayAddrProtocol
	}

	// 解析普通地址的协议
	// 格式：/ip4/.../udp/.../quic-v1/... 或 /ip4/.../tcp/...
	parts := strings.Split(addr, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		switch part {
		case "quic-v1", "quic", "tcp", "udp", "webtransport", "webrtc":
			return part
		}
	}

	return ""
}

