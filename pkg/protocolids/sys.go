package protocolids

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// 协议前缀常量（用于运行时判断协议范围）
// ============================================================================

// SysPrefix 系统协议前缀，所有系统协议以此开头
const SysPrefix = "/dep2p/sys/"

// AppPrefix 应用协议前缀，所有应用协议以此开头
const AppPrefix = "/dep2p/app/"

// ============================================================================
// 系统协议 ID（/dep2p/sys/...）
// 系统协议无需 RealmAuth 验证，是 DeP2P 基础设施的一部分
// ============================================================================

// ----------------------------------------------------------------------------
// 核心基础协议
// ----------------------------------------------------------------------------

// SysPing Ping 协议，用于节点存活检测和延迟测量
const SysPing types.ProtocolID = "/dep2p/sys/ping/1.0.0"

// SysGoodbye Goodbye 协议，用于优雅关闭连接通知
const SysGoodbye types.ProtocolID = "/dep2p/sys/goodbye/1.0.0"

// SysHeartbeat Heartbeat 协议，用于连接保活
const SysHeartbeat types.ProtocolID = "/dep2p/sys/heartbeat/1.0.0"

// SysIdentify 身份识别协议，用于交换节点元数据
const SysIdentify types.ProtocolID = "/dep2p/sys/identify/1.0.0"

// SysIdentifyPush 身份推送协议，用于主动推送身份变更
const SysIdentifyPush types.ProtocolID = "/dep2p/sys/identify/push/1.0.0"

// SysEcho Echo 协议，用于基础连接测试
const SysEcho types.ProtocolID = "/dep2p/sys/echo/1.0.0"

// ----------------------------------------------------------------------------
// 安全协议
// ----------------------------------------------------------------------------

// SysNoise Noise 协议，用于安全握手和加密通道建立
const SysNoise types.ProtocolID = "/dep2p/sys/noise/1.0.0"

// ----------------------------------------------------------------------------
// 发现协议
// ----------------------------------------------------------------------------

// SysDHT DHT 协议，用于分布式节点发现和路由
const SysDHT types.ProtocolID = "/dep2p/sys/dht/1.0.0"

// SysRendezvous Rendezvous 协议，用于节点会合发现
const SysRendezvous types.ProtocolID = "/dep2p/sys/rendezvous/1.0.0"

// ----------------------------------------------------------------------------
// 中继与 NAT 穿透协议
// ----------------------------------------------------------------------------

// SysRelay 中继协议主协议，用于中继连接协商
const SysRelay types.ProtocolID = "/dep2p/sys/relay/1.0.0"

// SysRelayHop 中继跳转协议，用于中继数据转发
const SysRelayHop types.ProtocolID = "/dep2p/sys/relay/hop/1.0.0"

// SysRelayStop 中继停止协议，用于终止中继连接
const SysRelayStop types.ProtocolID = "/dep2p/sys/relay/stop/1.0.0"

// SysHolepunch 打洞协议，用于 NAT 穿透
const SysHolepunch types.ProtocolID = "/dep2p/sys/holepunch/1.0.0"

// SysReachability 可达性验证协议（dial-back），用于探测节点公网可达性
const SysReachability types.ProtocolID = "/dep2p/sys/reachability/1.0.0"

// SysReachabilityWitness 入站见证协议，用于无外部依赖的 VerifiedDirect 升级
// 当 peer 使用候选地址成功连入后，自动发送见证报告；
// 同一候选地址被 >= MinWitnesses 个不同 IP 前缀的 peer 见证后，升级为 VerifiedDirect
const SysReachabilityWitness types.ProtocolID = "/dep2p/sys/reachability-witness/1.0.0"

// SysDeviceID 设备身份协议（可选）：多设备身份声明与证书交换
const SysDeviceID types.ProtocolID = "/dep2p/sys/device-id/1.0.0"

// ----------------------------------------------------------------------------
// 地址管理协议
// ----------------------------------------------------------------------------

// SysAddrMgmt 地址管理协议，用于地址通告和签名记录交换
const SysAddrMgmt types.ProtocolID = "/dep2p/sys/addr-mgmt/1.0.0"

// SysAddrExchange 地址交换协议，用于中继场景下的地址协商
const SysAddrExchange types.ProtocolID = "/dep2p/sys/addr-exchange/1.0.0"

// ----------------------------------------------------------------------------
// Realm 协议
// ----------------------------------------------------------------------------

// SysRealmAuth Realm 认证协议，用于连接级 Realm 成员验证
const SysRealmAuth types.ProtocolID = "/dep2p/sys/realm/auth/1.0.0"

// SysRealmSync Realm 同步协议，用于 Realm 状态同步
const SysRealmSync types.ProtocolID = "/dep2p/sys/realm/sync/1.0.0"

// ----------------------------------------------------------------------------
// 消息传递协议（系统级）
// ----------------------------------------------------------------------------

// SysGossipSub GossipSub 协议，用于 Pub/Sub 消息传递
const SysGossipSub types.ProtocolID = "/dep2p/sys/gossipsub/1.1.0"

// ----------------------------------------------------------------------------
// 测试专用协议（系统级，仅测试环境使用）
// ----------------------------------------------------------------------------

// SysTestEcho 测试用 Echo 协议
const SysTestEcho types.ProtocolID = "/dep2p/sys/test/echo/1.0.0"

// SysTestBackpressure 测试用背压协议
const SysTestBackpressure types.ProtocolID = "/dep2p/sys/test/backpressure/1.0.0"

// ============================================================================
// 协议命名空间（IMPL-1227 新增）
// ============================================================================

// RealmPrefix Realm 协议前缀模板
// 格式: /dep2p/realm/<realmID>/...
// 用于 Realm 内部控制协议（成员同步、发现等）
const RealmPrefix = "/dep2p/realm/"

// RealmProtocolTemplate Realm 协议完整模板
const RealmProtocolTemplate = "/dep2p/realm/%s/"

// AppProtocolTemplate 应用协议完整模板
// 格式: /dep2p/app/<realmID>/...
// 用于用户自定义的应用层协议
const AppProtocolTemplate = "/dep2p/app/%s/"

// ============================================================================
// 协议命名空间错误
// ============================================================================

var (
	// ErrReservedProtocol 保留协议前缀错误
	// 当用户尝试注册 /dep2p/sys/* 或 /dep2p/realm/* 协议时返回
	ErrReservedProtocol = errors.New("protocol uses reserved prefix")

	// ErrCrossRealmProtocol 跨 Realm 协议错误
	// 当用户尝试使用其他 Realm 的协议时返回
	ErrCrossRealmProtocol = errors.New("cross-realm protocol not allowed")

	// ErrNoRealmInProtocol 协议中无 RealmID 错误
	// 当从非 Realm 协议中提取 RealmID 时返回
	ErrNoRealmInProtocol = errors.New("no realm ID in protocol")

	// ErrInvalidProtocolFormat 无效的协议格式错误
	ErrInvalidProtocolFormat = errors.New("invalid protocol format")
)

// ============================================================================
// 协议命名空间函数
// ============================================================================

// FullAppProtocol 生成完整的应用协议 ID
//
// 用户只需指定相对协议名（如 "chat/1.0.0"），框架自动补全为：
// /dep2p/app/<realmID>/chat/1.0.0
//
// 示例:
//
//	proto := FullAppProtocol(realmID, "chat/1.0.0")
//	// 返回: /dep2p/app/abc123.../chat/1.0.0
func FullAppProtocol(realmID types.RealmID, userProto string) types.ProtocolID {
	return types.ProtocolID(fmt.Sprintf("/dep2p/app/%s/%s", realmID, userProto))
}

// FullRealmProtocol 生成完整的 Realm 协议 ID
//
// 用于 Realm 内部控制协议（成员同步、发现等），由框架内部使用。
//
// 示例:
//
//	proto := FullRealmProtocol(realmID, "membership/1.0.0")
//	// 返回: /dep2p/realm/abc123.../membership/1.0.0
func FullRealmProtocol(realmID types.RealmID, subProto string) types.ProtocolID {
	return types.ProtocolID(fmt.Sprintf("/dep2p/realm/%s/%s", realmID, subProto))
}

// ValidateUserProtocol 验证用户协议是否合法
//
// 检查规则：
//   - 不能以 /dep2p/sys/ 开头（系统协议保留）
//   - 不能以 /dep2p/realm/ 开头（Realm 控制协议保留）
//   - 不能以 /dep2p/app/ 开头（由框架自动添加）
//   - 如果包含完整路径且 RealmID 不匹配，返回跨 Realm 错误
//
// 用户应只提供相对协议名，如 "chat/1.0.0"。
func ValidateUserProtocol(proto string, currentRealmID types.RealmID) error {
	// 检查系统协议前缀
	if strings.HasPrefix(proto, SysPrefix) {
		return ErrReservedProtocol
	}

	// 检查 Realm 协议前缀（用户不能直接指定）
	if strings.HasPrefix(proto, RealmPrefix) {
		return ErrReservedProtocol
	}

	// 检查应用协议前缀（用户不能直接指定完整路径）
	if strings.HasPrefix(proto, AppPrefix) {
		// 进一步检查：如果包含其他 RealmID，拒绝
		extractedRealmID, err := ExtractRealmID(types.ProtocolID(proto))
		if err == nil && extractedRealmID != currentRealmID {
			return ErrCrossRealmProtocol
		}
		return ErrReservedProtocol
	}

	return nil
}

// ExtractRealmID 从协议 ID 中提取 RealmID
//
// 支持从以下格式中提取：
//   - /dep2p/app/<realmID>/...
//   - /dep2p/realm/<realmID>/...
//
// 如果协议不包含 RealmID（如系统协议），返回 ErrNoRealmInProtocol。
func ExtractRealmID(proto types.ProtocolID) (types.RealmID, error) {
	s := string(proto)

	// 检查应用协议前缀
	if strings.HasPrefix(s, AppPrefix) {
		remaining := s[len(AppPrefix):]
		parts := strings.SplitN(remaining, "/", 2)
		if len(parts) >= 1 && parts[0] != "" {
			return types.RealmID(parts[0]), nil
		}
		return "", ErrInvalidProtocolFormat
	}

	// 检查 Realm 协议前缀
	if strings.HasPrefix(s, RealmPrefix) {
		remaining := s[len(RealmPrefix):]
		parts := strings.SplitN(remaining, "/", 2)
		if len(parts) >= 1 && parts[0] != "" {
			return types.RealmID(parts[0]), nil
		}
		return "", ErrInvalidProtocolFormat
	}

	return "", ErrNoRealmInProtocol
}

// IsSystemProtocol 检查协议是否为系统协议
//
// 系统协议以 /dep2p/sys/ 开头，不绑定任何 Realm，全网通用。
func IsSystemProtocol(proto types.ProtocolID) bool {
	return strings.HasPrefix(string(proto), SysPrefix)
}

// IsRealmProtocol 检查协议是否为 Realm 控制协议
//
// Realm 控制协议以 /dep2p/realm/<realmID>/ 开头，用于 Realm 内部控制。
func IsRealmProtocol(proto types.ProtocolID) bool {
	return strings.HasPrefix(string(proto), RealmPrefix)
}

// IsAppProtocol 检查协议是否为应用协议
//
// 应用协议以 /dep2p/app/<realmID>/ 开头，用于用户自定义业务。
func IsAppProtocol(proto types.ProtocolID) bool {
	return strings.HasPrefix(string(proto), AppPrefix)
}

// BelongsToRealm 检查协议是否属于指定 Realm
//
// 只有应用协议和 Realm 控制协议可以属于某个 Realm。
// 系统协议不属于任何 Realm。
func BelongsToRealm(proto types.ProtocolID, realmID types.RealmID) bool {
	extractedID, err := ExtractRealmID(proto)
	if err != nil {
		return false
	}
	return extractedID == realmID
}

