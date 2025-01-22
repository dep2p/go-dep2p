package rcmgr

import (
	"encoding/json"
	"math"
	"strconv"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"

	"github.com/pbnjay/memory"
)

// baseLimitConfig 基础限制配置结构体
type baseLimitConfig struct {
	BaseLimit         BaseLimit         // 基础限制
	BaseLimitIncrease BaseLimitIncrease // 基础限制增量
}

// ScalingLimitConfig 是用于配置默认限制的结构体。
// {}BaseLimit 是适用于最小节点的限制(dep2p 使用 128MB 内存)和 256 个文件描述符。
// {}LimitIncrease 是每增加 1GB RAM 时额外授予的限制。
type ScalingLimitConfig struct {
	SystemBaseLimit     BaseLimit         // 系统基础限制
	SystemLimitIncrease BaseLimitIncrease // 系统限制增量

	TransientBaseLimit     BaseLimit         // 临时基础限制
	TransientLimitIncrease BaseLimitIncrease // 临时限制增量

	AllowlistedSystemBaseLimit     BaseLimit         // 白名单系统基础限制
	AllowlistedSystemLimitIncrease BaseLimitIncrease // 白名单系统限制增量

	AllowlistedTransientBaseLimit     BaseLimit         // 白名单临时基础限制
	AllowlistedTransientLimitIncrease BaseLimitIncrease // 白名单临时限制增量

	ServiceBaseLimit     BaseLimit                  // 服务基础限制
	ServiceLimitIncrease BaseLimitIncrease          // 服务限制增量
	ServiceLimits        map[string]baseLimitConfig // 使用 AddServiceLimit 修改

	ServicePeerBaseLimit     BaseLimit                  // 服务对等节点基础限制
	ServicePeerLimitIncrease BaseLimitIncrease          // 服务对等节点限制增量
	ServicePeerLimits        map[string]baseLimitConfig // 使用 AddServicePeerLimit 修改

	ProtocolBaseLimit     BaseLimit                       // 协议基础限制
	ProtocolLimitIncrease BaseLimitIncrease               // 协议限制增量
	ProtocolLimits        map[protocol.ID]baseLimitConfig // 使用 AddProtocolLimit 修改

	ProtocolPeerBaseLimit     BaseLimit                       // 协议对等节点基础限制
	ProtocolPeerLimitIncrease BaseLimitIncrease               // 协议对等节点限制增量
	ProtocolPeerLimits        map[protocol.ID]baseLimitConfig // 使用 AddProtocolPeerLimit 修改

	PeerBaseLimit     BaseLimit                   // 对等节点基础限制
	PeerLimitIncrease BaseLimitIncrease           // 对等节点限制增量
	PeerLimits        map[peer.ID]baseLimitConfig // 使用 AddPeerLimit 修改

	ConnBaseLimit     BaseLimit         // 连接基础限制
	ConnLimitIncrease BaseLimitIncrease // 连接限制增量

	StreamBaseLimit     BaseLimit         // 流基础限制
	StreamLimitIncrease BaseLimitIncrease // 流限制增量
}

// AddServiceLimit 添加服务限制
// 参数:
//   - svc: string - 服务名称
//   - base: BaseLimit - 基础限制
//   - inc: BaseLimitIncrease - 限制增量
func (cfg *ScalingLimitConfig) AddServiceLimit(svc string, base BaseLimit, inc BaseLimitIncrease) {
	// 如果服务限制映射为空,则初始化
	if cfg.ServiceLimits == nil {
		cfg.ServiceLimits = make(map[string]baseLimitConfig)
	}
	// 添加服务限制配置
	cfg.ServiceLimits[svc] = baseLimitConfig{
		BaseLimit:         base,
		BaseLimitIncrease: inc,
	}
}

// AddProtocolLimit 添加协议限制
// 参数:
//   - proto: protocol.ID - 协议ID
//   - base: BaseLimit - 基础限制
//   - inc: BaseLimitIncrease - 限制增量
func (cfg *ScalingLimitConfig) AddProtocolLimit(proto protocol.ID, base BaseLimit, inc BaseLimitIncrease) {
	// 如果协议限制映射为空,则初始化
	if cfg.ProtocolLimits == nil {
		cfg.ProtocolLimits = make(map[protocol.ID]baseLimitConfig)
	}
	// 添加协议限制配置
	cfg.ProtocolLimits[proto] = baseLimitConfig{
		BaseLimit:         base,
		BaseLimitIncrease: inc,
	}
}

// AddPeerLimit 添加对等节点限制
// 参数:
//   - p: peer.ID - 对等节点ID
//   - base: BaseLimit - 基础限制
//   - inc: BaseLimitIncrease - 限制增量
func (cfg *ScalingLimitConfig) AddPeerLimit(p peer.ID, base BaseLimit, inc BaseLimitIncrease) {
	// 如果对等节点限制映射为空,则初始化
	if cfg.PeerLimits == nil {
		cfg.PeerLimits = make(map[peer.ID]baseLimitConfig)
	}
	// 添加对等节点限制配置
	cfg.PeerLimits[p] = baseLimitConfig{
		BaseLimit:         base,
		BaseLimitIncrease: inc,
	}
}

// AddServicePeerLimit 添加服务对等节点限制
// 参数:
//   - svc: string - 服务名称
//   - base: BaseLimit - 基础限制
//   - inc: BaseLimitIncrease - 限制增量
func (cfg *ScalingLimitConfig) AddServicePeerLimit(svc string, base BaseLimit, inc BaseLimitIncrease) {
	// 如果服务对等节点限制映射为空,则初始化
	if cfg.ServicePeerLimits == nil {
		cfg.ServicePeerLimits = make(map[string]baseLimitConfig)
	}
	// 添加服务对等节点限制配置
	cfg.ServicePeerLimits[svc] = baseLimitConfig{
		BaseLimit:         base,
		BaseLimitIncrease: inc,
	}
}

// AddProtocolPeerLimit 添加协议对等节点限制
// 参数:
//   - proto: protocol.ID - 协议ID
//   - base: BaseLimit - 基础限制
//   - inc: BaseLimitIncrease - 限制增量
func (cfg *ScalingLimitConfig) AddProtocolPeerLimit(proto protocol.ID, base BaseLimit, inc BaseLimitIncrease) {
	// 如果协议对等节点限制映射为空,则初始化
	if cfg.ProtocolPeerLimits == nil {
		cfg.ProtocolPeerLimits = make(map[protocol.ID]baseLimitConfig)
	}
	// 添加协议对等节点限制配置
	cfg.ProtocolPeerLimits[proto] = baseLimitConfig{
		BaseLimit:         base,
		BaseLimitIncrease: inc,
	}
}

// LimitVal 限制值类型
type LimitVal int

const (
	// DefaultLimit 是资源的默认值。具体值取决于上下文,但会从 `DefaultLimits` 获取值。
	DefaultLimit LimitVal = 0
	// Unlimited 是无限资源的值。任意高的数字也可以。
	Unlimited LimitVal = -1
	// BlockAllLimit 是不允许任何资源的 LimitVal。
	BlockAllLimit LimitVal = -2
)

// MarshalJSON 实现 JSON 序列化
// 参数:
//   - 无
//
// 返回值:
//   - []byte - 序列化后的JSON字节数组
//   - error 错误信息
func (l LimitVal) MarshalJSON() ([]byte, error) {
	// 如果是无限制值,返回"unlimited"字符串
	if l == Unlimited {
		log.Debugf("序列化无限制值")
		return json.Marshal("unlimited")
		// 如果是默认限制值,返回"default"字符串
	} else if l == DefaultLimit {
		log.Debugf("序列化默认限制值")
		return json.Marshal("default")
		// 如果是阻止所有限制值,返回"blockAll"字符串
	} else if l == BlockAllLimit {
		log.Debugf("序列化阻止所有限制值")
		return json.Marshal("blockAll")
	}
	// 其他情况将限制值转换为整数返回
	return json.Marshal(int(l))
}

// UnmarshalJSON 实现 JSON 反序列化
// 参数:
//   - b: []byte - 要反序列化的JSON字节数组
//
// 返回值:
//   - error 错误信息
func (l *LimitVal) UnmarshalJSON(b []byte) error {
	// 如果是"default"字符串,设置为默认限制值
	if string(b) == `"default"` {
		*l = DefaultLimit
		return nil
		// 如果是"unlimited"字符串,设置为无限制值
	} else if string(b) == `"unlimited"` {
		*l = Unlimited
		return nil
		// 如果是"blockAll"字符串,设置为阻止所有限制值
	} else if string(b) == `"blockAll"` {
		*l = BlockAllLimit
		return nil
	}

	// 尝试将JSON解析为整数值
	var val int
	if err := json.Unmarshal(b, &val); err != nil {
		log.Debugf("反序列化限制值失败: %v", err)
		return err
	}

	// 如果值为0,将其解释为阻止所有限制
	if val == 0 {
		*l = BlockAllLimit
		return nil
	}

	// 将整数值转换为LimitVal类型
	*l = LimitVal(val)
	return nil
}

// Build 构建限制值
// 参数:
//   - defaultVal: int - 默认限制值
//
// 返回值:
//   - int - 构建后的实际限制值
func (l LimitVal) Build(defaultVal int) int {
	// 如果是默认限制值,返回传入的默认值
	if l == DefaultLimit {
		return defaultVal
	}
	// 如果是无限制值,返回最大整数值
	if l == Unlimited {
		return math.MaxInt
	}
	// 如果是阻止所有限制值,返回0
	if l == BlockAllLimit {
		return 0
	}
	// 其他情况将LimitVal转换为int返回
	return int(l)
}

// LimitVal64 64位限制值类型
type LimitVal64 int64

const (
	// DefaultLimit64 是资源的默认值。
	DefaultLimit64 LimitVal64 = 0
	// Unlimited64 是无限资源的值。
	Unlimited64 LimitVal64 = -1
	// BlockAllLimit64 是不允许任何资源的 LimitVal。
	BlockAllLimit64 LimitVal64 = -2
)

// MarshalJSON 实现 JSON 序列化
// 返回值:
//   - []byte - 序列化后的 JSON 字节数组
//   - error 序列化过程中的错误
func (l LimitVal64) MarshalJSON() ([]byte, error) {
	// 如果是无限制值,序列化为 "unlimited" 字符串
	if l == Unlimited64 {
		log.Debugf("序列化无限制值")
		return json.Marshal("unlimited")
	} else if l == DefaultLimit64 { // 如果是默认限制值,序列化为 "default" 字符串
		log.Debugf("序列化默认限制值")
		return json.Marshal("default")
	} else if l == BlockAllLimit64 { // 如果是阻止所有限制值,序列化为 "blockAll" 字符串
		log.Debugf("序列化阻止所有限制值")
		return json.Marshal("blockAll")
	}

	// 将其转换为字符串,因为 JSON 不支持 64 位整数
	return json.Marshal(strconv.FormatInt(int64(l), 10))
}

// UnmarshalJSON 实现 JSON 反序列化
// 参数:
//   - b: []byte - 要反序列化的 JSON 字节数组
//
// 返回值:
//   - error 反序列化过程中的错误
func (l *LimitVal64) UnmarshalJSON(b []byte) error {
	// 如果是 "default" 字符串,设置为默认限制值
	if string(b) == `"default"` {
		log.Debugf("反序列化默认限制值")
		*l = DefaultLimit64
		return nil
	} else if string(b) == `"unlimited"` { // 如果是 "unlimited" 字符串,设置为无限制值
		log.Debugf("反序列化无限制值")
		*l = Unlimited64
		return nil
	} else if string(b) == `"blockAll"` { // 如果是 "blockAll" 字符串,设置为阻止所有限制值
		log.Debugf("反序列化阻止所有限制值")
		*l = BlockAllLimit64
		return nil
	}

	// 尝试解析为字符串
	var val string
	if err := json.Unmarshal(b, &val); err != nil {
		// 如果解析字符串失败,尝试解析为整数(向后兼容)
		var val int
		if err := json.Unmarshal(b, &val); err != nil {
			log.Debugf("反序列化限制值失败: %v", err)
			return err
		}

		// 如果 JSON 中有明确的 0,解释为阻止所有
		if val == 0 {
			log.Debugf("反序列化阻止所有限制值")
			*l = BlockAllLimit64
			return nil
		}

		// 将整数值转换为 LimitVal64
		*l = LimitVal64(val)
		return nil
	}

	// 将字符串解析为 64 位整数
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		log.Debugf("反序列化限制值失败: %v", err)
		return err
	}

	// 如果值为 0,解释为阻止所有
	if i == 0 {
		// 如果 JSON 中有明确的 0,我们应该将其解释为阻止所有。
		*l = BlockAllLimit64
		return nil
	}

	// 将解析的整数值转换为 LimitVal64
	*l = LimitVal64(i)
	return nil
}

// Build 构建限制值
// 参数:
//   - defaultVal: int64 - 默认限制值
//
// 返回值:
//   - int64 - 构建后的实际限制值
func (l LimitVal64) Build(defaultVal int64) int64 {
	// 如果是默认限制值,返回传入的默认值
	if l == DefaultLimit64 {
		return defaultVal
	}
	// 如果是无限制值,返回最大 64 位整数值
	if l == Unlimited64 {
		return math.MaxInt64
	}
	// 如果是阻止所有限制值,返回 0
	if l == BlockAllLimit64 {
		return 0
	}
	// 其他情况将 LimitVal64 转换为 int64 返回
	return int64(l)
}

// ResourceLimits 是基本资源限制的类型。
type ResourceLimits struct {
	Streams         LimitVal   `json:",omitempty"` // 流限制
	StreamsInbound  LimitVal   `json:",omitempty"` // 入站流限制
	StreamsOutbound LimitVal   `json:",omitempty"` // 出站流限制
	Conns           LimitVal   `json:",omitempty"` // 连接限制
	ConnsInbound    LimitVal   `json:",omitempty"` // 入站连接限制
	ConnsOutbound   LimitVal   `json:",omitempty"` // 出站连接限制
	FD              LimitVal   `json:",omitempty"` // 文件描述符限制
	Memory          LimitVal64 `json:",omitempty"` // 内存限制
}

// IsDefault 检查资源限制是否为默认值
// 返回值:
//   - bool - 如果所有限制都是默认值则返回 true,否则返回 false
func (l *ResourceLimits) IsDefault() bool {
	// 如果指针为空,则视为默认值
	if l == nil {
		return true
	}

	// 检查所有字段是否都是默认值
	if l.Streams == DefaultLimit &&
		l.StreamsInbound == DefaultLimit &&
		l.StreamsOutbound == DefaultLimit &&
		l.Conns == DefaultLimit &&
		l.ConnsInbound == DefaultLimit &&
		l.ConnsOutbound == DefaultLimit &&
		l.FD == DefaultLimit &&
		l.Memory == DefaultLimit64 {
		return true
	}
	return false
}

// ToMaybeNilPtr 将资源限制转换为可能为 nil 的指针
// 返回值:
//   - *ResourceLimits - 如果是默认值则返回 nil,否则返回当前对象的指针
func (l *ResourceLimits) ToMaybeNilPtr() *ResourceLimits {
	// 如果是默认值则返回 nil
	if l.IsDefault() {
		return nil
	}
	// 否则返回当前对象的指针
	return l
}

// Apply 用另一个资源限制对象的值覆盖当前对象中的默认限制
// 参数:
//   - l2: ResourceLimits - 用于覆盖的资源限制对象
func (l *ResourceLimits) Apply(l2 ResourceLimits) {
	// 如果当前流限制是默认值,则使用 l2 的值
	if l.Streams == DefaultLimit {
		l.Streams = l2.Streams
	}
	// 如果当前入站流限制是默认值,则使用 l2 的值
	if l.StreamsInbound == DefaultLimit {
		l.StreamsInbound = l2.StreamsInbound
	}
	// 如果当前出站流限制是默认值,则使用 l2 的值
	if l.StreamsOutbound == DefaultLimit {
		l.StreamsOutbound = l2.StreamsOutbound
	}
	// 如果当前连接限制是默认值,则使用 l2 的值
	if l.Conns == DefaultLimit {
		l.Conns = l2.Conns
	}
	// 如果当前入站连接限制是默认值,则使用 l2 的值
	if l.ConnsInbound == DefaultLimit {
		l.ConnsInbound = l2.ConnsInbound
	}
	// 如果当前出站连接限制是默认值,则使用 l2 的值
	if l.ConnsOutbound == DefaultLimit {
		l.ConnsOutbound = l2.ConnsOutbound
	}
	// 如果当前文件描述符限制是默认值,则使用 l2 的值
	if l.FD == DefaultLimit {
		l.FD = l2.FD
	}
	// 如果当前内存限制是默认值,则使用 l2 的值
	if l.Memory == DefaultLimit64 {
		l.Memory = l2.Memory
	}
}

// Build 根据默认值构建基础限制
// 参数:
//   - defaults: Limit - 默认限制值,用于提供默认的资源限制配置
//
// 返回值:
//   - BaseLimit - 构建的基础限制对象,包含所有资源限制的具体值
func (l *ResourceLimits) Build(defaults Limit) BaseLimit {
	// 如果当前对象为 nil,则使用默认值构建基础限制
	if l == nil {
		return BaseLimit{
			Streams:         defaults.GetStreamTotalLimit(),               // 获取默认的总流限制
			StreamsInbound:  defaults.GetStreamLimit(network.DirInbound),  // 获取默认的入站流限制
			StreamsOutbound: defaults.GetStreamLimit(network.DirOutbound), // 获取默认的出站流限制
			Conns:           defaults.GetConnTotalLimit(),                 // 获取默认的总连接限制
			ConnsInbound:    defaults.GetConnLimit(network.DirInbound),    // 获取默认的入站连接限制
			ConnsOutbound:   defaults.GetConnLimit(network.DirOutbound),   // 获取默认的出站连接限制
			FD:              defaults.GetFDLimit(),                        // 获取默认的文件描述符限制
			Memory:          defaults.GetMemoryLimit(),                    // 获取默认的内存限制
		}
	}

	// 使用当前值和默认值构建基础限制
	return BaseLimit{
		Streams:         l.Streams.Build(defaults.GetStreamTotalLimit()),                       // 构建流总限制
		StreamsInbound:  l.StreamsInbound.Build(defaults.GetStreamLimit(network.DirInbound)),   // 构建入站流限制
		StreamsOutbound: l.StreamsOutbound.Build(defaults.GetStreamLimit(network.DirOutbound)), // 构建出站流限制
		Conns:           l.Conns.Build(defaults.GetConnTotalLimit()),                           // 构建连接总限制
		ConnsInbound:    l.ConnsInbound.Build(defaults.GetConnLimit(network.DirInbound)),       // 构建入站连接限制
		ConnsOutbound:   l.ConnsOutbound.Build(defaults.GetConnLimit(network.DirOutbound)),     // 构建出站连接限制
		FD:              l.FD.Build(defaults.GetFDLimit()),                                     // 构建文件描述符限制
		Memory:          l.Memory.Build(defaults.GetMemoryLimit()),                             // 构建内存限制
	}
}

// PartialLimitConfig 定义部分限制配置
type PartialLimitConfig struct {
	System    ResourceLimits `json:",omitempty"` // 系统级别的资源限制
	Transient ResourceLimits `json:",omitempty"` // 临时资源限制

	// 应用于具有白名单多地址的资源的限制。
	// 这些限制仅在达到正常的系统和临时限制时使用。
	AllowlistedSystem    ResourceLimits `json:",omitempty"` // 白名单系统资源限制
	AllowlistedTransient ResourceLimits `json:",omitempty"` // 白名单临时资源限制

	ServiceDefault ResourceLimits            `json:",omitempty"` // 服务默认资源限制
	Service        map[string]ResourceLimits `json:",omitempty"` // 特定服务的资源限制映射

	ServicePeerDefault ResourceLimits            `json:",omitempty"` // 服务对等节点默认资源限制
	ServicePeer        map[string]ResourceLimits `json:",omitempty"` // 特定服务对等节点的资源限制映射

	ProtocolDefault ResourceLimits                 `json:",omitempty"` // 协议默认资源限制
	Protocol        map[protocol.ID]ResourceLimits `json:",omitempty"` // 特定协议的资源限制映射

	ProtocolPeerDefault ResourceLimits                 `json:",omitempty"` // 协议对等节点默认资源限制
	ProtocolPeer        map[protocol.ID]ResourceLimits `json:",omitempty"` // 特定协议对等节点的资源限制映射

	PeerDefault ResourceLimits             `json:",omitempty"` // 对等节点默认资源限制
	Peer        map[peer.ID]ResourceLimits `json:",omitempty"` // 特定对等节点的资源限制映射

	Conn   ResourceLimits `json:",omitempty"` // 连接资源限制
	Stream ResourceLimits `json:",omitempty"` // 流资源限制
}

// MarshalJSON 实现 JSON 序列化
// 返回值:
//   - []byte - 序列化后的 JSON 字节数组
//   - error 序列化过程中的错误
func (cfg *PartialLimitConfig) MarshalJSON() ([]byte, error) {
	// 将编码的对等点 ID 进行序列化
	encodedPeerMap := make(map[string]ResourceLimits, len(cfg.Peer)) // 创建编码后的对等节点映射
	for p, v := range cfg.Peer {
		encodedPeerMap[p.String()] = v // 将对等节点 ID 转换为字符串作为键
	}

	type Alias PartialLimitConfig // 创建类型别名以避免递归调用
	return json.Marshal(&struct {
		*Alias
		// 使用字符串类型以便正确序列化对等点 ID
		Peer map[string]ResourceLimits `json:",omitempty"` // 序列化后的对等节点映射

		// 其余字段使用指针以便在序列化结果中省略空值
		System               *ResourceLimits `json:",omitempty"` // 系统资源限制指针
		Transient            *ResourceLimits `json:",omitempty"` // 临时资源限制指针
		AllowlistedSystem    *ResourceLimits `json:",omitempty"` // 白名单系统资源限制指针
		AllowlistedTransient *ResourceLimits `json:",omitempty"` // 白名单临时资源限制指针

		ServiceDefault *ResourceLimits `json:",omitempty"` // 服务默认资源限制指针

		ServicePeerDefault *ResourceLimits `json:",omitempty"` // 服务对等节点默认资源限制指针

		ProtocolDefault *ResourceLimits `json:",omitempty"` // 协议默认资源限制指针

		ProtocolPeerDefault *ResourceLimits `json:",omitempty"` // 协议对等节点默认资源限制指针

		PeerDefault *ResourceLimits `json:",omitempty"` // 对等节点默认资源限制指针

		Conn   *ResourceLimits `json:",omitempty"` // 连接资源限制指针
		Stream *ResourceLimits `json:",omitempty"` // 流资源限制指针
	}{
		Alias: (*Alias)(cfg),  // 转换为别名类型
		Peer:  encodedPeerMap, // 使用编码后的对等节点映射

		System:               cfg.System.ToMaybeNilPtr(),               // 转换系统限制为可能为空的指针
		Transient:            cfg.Transient.ToMaybeNilPtr(),            // 转换临时限制为可能为空的指针
		AllowlistedSystem:    cfg.AllowlistedSystem.ToMaybeNilPtr(),    // 转换白名单系统限制为可能为空的指针
		AllowlistedTransient: cfg.AllowlistedTransient.ToMaybeNilPtr(), // 转换白名单临时限制为可能为空的指针
		ServiceDefault:       cfg.ServiceDefault.ToMaybeNilPtr(),       // 转换服务默认限制为可能为空的指针
		ServicePeerDefault:   cfg.ServicePeerDefault.ToMaybeNilPtr(),   // 转换服务对等节点默认限制为可能为空的指针
		ProtocolDefault:      cfg.ProtocolDefault.ToMaybeNilPtr(),      // 转换协议默认限制为可能为空的指针
		ProtocolPeerDefault:  cfg.ProtocolPeerDefault.ToMaybeNilPtr(),  // 转换协议对等节点默认限制为可能为空的指针
		PeerDefault:          cfg.PeerDefault.ToMaybeNilPtr(),          // 转换对等节点默认限制为可能为空的指针
		Conn:                 cfg.Conn.ToMaybeNilPtr(),                 // 转换连接限制为可能为空的指针
		Stream:               cfg.Stream.ToMaybeNilPtr(),               // 转换流限制为可能为空的指针
	})
}

// applyResourceLimitsMap 应用资源限制映射
// 参数:
//   - this: *map[K]ResourceLimits - 目标资源限制映射的指针
//   - other: map[K]ResourceLimits - 源资源限制映射
//   - fallbackDefault: ResourceLimits - 默认的资源限制
func applyResourceLimitsMap[K comparable](this *map[K]ResourceLimits, other map[K]ResourceLimits, fallbackDefault ResourceLimits) {
	// 遍历目标映射中的所有键值对
	for k, l := range *this {
		r := fallbackDefault        // 使用默认限制作为基础
		if l2, ok := other[k]; ok { // 如果源映射中存在相同的键
			r = l2 // 使用源映射中的限制
		}
		l.Apply(r)     // 应用限制
		(*this)[k] = l // 更新目标映射
	}
	// 如果目标映射为空且源映射不为空
	if *this == nil && other != nil {
		*this = make(map[K]ResourceLimits) // 初始化目标映射
	}
	// 遍历源映射中的所有键值对
	for k, l := range other {
		if _, ok := (*this)[k]; !ok { // 如果目标映射中不存在该键
			(*this)[k] = l // 添加到目标映射
		}
	}
}

// Apply 应用另一个配置
// 参数:
//   - c: PartialLimitConfig - 要应用的配置
func (cfg *PartialLimitConfig) Apply(c PartialLimitConfig) {
	// 应用系统限制
	cfg.System.Apply(c.System)
	// 应用临时限制
	cfg.Transient.Apply(c.Transient)
	// 应用白名单系统限制
	cfg.AllowlistedSystem.Apply(c.AllowlistedSystem)
	// 应用白名单临时限制
	cfg.AllowlistedTransient.Apply(c.AllowlistedTransient)
	// 应用服务默认限制
	cfg.ServiceDefault.Apply(c.ServiceDefault)
	// 应用服务对等节点默认限制
	cfg.ServicePeerDefault.Apply(c.ServicePeerDefault)
	// 应用协议默认限制
	cfg.ProtocolDefault.Apply(c.ProtocolDefault)
	// 应用协议对等节点默认限制
	cfg.ProtocolPeerDefault.Apply(c.ProtocolPeerDefault)
	// 应用对等节点默认限制
	cfg.PeerDefault.Apply(c.PeerDefault)
	// 应用连接限制
	cfg.Conn.Apply(c.Conn)
	// 应用流限制
	cfg.Stream.Apply(c.Stream)

	// 应用服务限制映射
	applyResourceLimitsMap(&cfg.Service, c.Service, cfg.ServiceDefault)
	// 应用服务对等节点限制映射
	applyResourceLimitsMap(&cfg.ServicePeer, c.ServicePeer, cfg.ServicePeerDefault)
	// 应用协议限制映射
	applyResourceLimitsMap(&cfg.Protocol, c.Protocol, cfg.ProtocolDefault)
	// 应用协议对等节点限制映射
	applyResourceLimitsMap(&cfg.ProtocolPeer, c.ProtocolPeer, cfg.ProtocolPeerDefault)
	// 应用对等节点限制映射
	applyResourceLimitsMap(&cfg.Peer, c.Peer, cfg.PeerDefault)
}

// Build 根据默认值构建具体限制配置
// 参数:
//   - defaults: ConcreteLimitConfig - 默认的具体限制配置
//
// 返回值:
//   - ConcreteLimitConfig - 构建后的具体限制配置
func (cfg PartialLimitConfig) Build(defaults ConcreteLimitConfig) ConcreteLimitConfig {
	// 使用默认配置作为基础
	out := defaults

	// 构建系统限制
	out.system = cfg.System.Build(defaults.system)
	// 构建临时限制
	out.transient = cfg.Transient.Build(defaults.transient)
	// 构建白名单系统限制
	out.allowlistedSystem = cfg.AllowlistedSystem.Build(defaults.allowlistedSystem)
	// 构建白名单临时限制
	out.allowlistedTransient = cfg.AllowlistedTransient.Build(defaults.allowlistedTransient)
	// 构建服务默认限制
	out.serviceDefault = cfg.ServiceDefault.Build(defaults.serviceDefault)
	// 构建服务对等节点默认限制
	out.servicePeerDefault = cfg.ServicePeerDefault.Build(defaults.servicePeerDefault)
	// 构建协议默认限制
	out.protocolDefault = cfg.ProtocolDefault.Build(defaults.protocolDefault)
	// 构建协议对等节点默认限制
	out.protocolPeerDefault = cfg.ProtocolPeerDefault.Build(defaults.protocolPeerDefault)
	// 构建对等节点默认限制
	out.peerDefault = cfg.PeerDefault.Build(defaults.peerDefault)
	// 构建连接限制
	out.conn = cfg.Conn.Build(defaults.conn)
	// 构建流限制
	out.stream = cfg.Stream.Build(defaults.stream)

	// 构建服务限制映射
	out.service = buildMapWithDefault(cfg.Service, defaults.service, out.serviceDefault)
	// 构建服务对等节点限制映射
	out.servicePeer = buildMapWithDefault(cfg.ServicePeer, defaults.servicePeer, out.servicePeerDefault)
	// 构建协议限制映射
	out.protocol = buildMapWithDefault(cfg.Protocol, defaults.protocol, out.protocolDefault)
	// 构建协议对等节点限制映射
	out.protocolPeer = buildMapWithDefault(cfg.ProtocolPeer, defaults.protocolPeer, out.protocolPeerDefault)
	// 构建对等节点限制映射
	out.peer = buildMapWithDefault(cfg.Peer, defaults.peer, out.peerDefault)

	return out
}

// buildMapWithDefault 使用默认值构建映射
// 参数:
//   - definedLimits: map[K]ResourceLimits - 已定义的限制映射
//   - defaults: map[K]BaseLimit - 默认限制映射
//   - fallbackDefault: BaseLimit - 回退默认限制
//
// 返回值:
//   - map[K]BaseLimit - 构建后的限制映射
func buildMapWithDefault[K comparable](definedLimits map[K]ResourceLimits, defaults map[K]BaseLimit, fallbackDefault BaseLimit) map[K]BaseLimit {
	// 如果定义的限制和默认限制都为空,返回 nil
	if definedLimits == nil && defaults == nil {
		return nil
	}

	// 创建输出映射
	out := make(map[K]BaseLimit)
	// 复制默认限制到输出映射
	for k, l := range defaults {
		out[k] = l
	}

	// 遍历定义的限制
	for k, l := range definedLimits {
		if defaultForKey, ok := out[k]; ok {
			// 如果存在默认值,使用它构建限制
			out[k] = l.Build(defaultForKey)
		} else {
			// 否则使用回退默认值构建限制
			out[k] = l.Build(fallbackDefault)
		}
	}

	return out
}

// ConcreteLimitConfig 类似于 PartialLimitConfig,但所有值都已定义。
// 没有未设置的"默认"值。通常通过调用 PartialLimitConfig.Build(rcmgr.DefaultLimits.AutoScale()) 构建
type ConcreteLimitConfig struct {
	system    BaseLimit // 系统级别的基础限制
	transient BaseLimit // 临时基础限制

	// 应用于具有白名单多地址的资源的限制。
	// 这些限制仅在达到正常的系统和临时限制时使用。
	allowlistedSystem    BaseLimit // 白名单系统基础限制
	allowlistedTransient BaseLimit // 白名单临时基础限制

	serviceDefault BaseLimit            // 服务默认基础限制
	service        map[string]BaseLimit // 特定服务的基础限制映射

	servicePeerDefault BaseLimit            // 服务对等节点默认基础限制
	servicePeer        map[string]BaseLimit // 特定服务对等节点的基础限制映射

	protocolDefault BaseLimit                 // 协议默认基础限制
	protocol        map[protocol.ID]BaseLimit // 特定协议的基础限制映射

	protocolPeerDefault BaseLimit                 // 协议对等节点默认基础限制
	protocolPeer        map[protocol.ID]BaseLimit // 特定协议对等节点的基础限制映射

	peerDefault BaseLimit             // 对等节点默认基础限制
	peer        map[peer.ID]BaseLimit // 特定对等节点的基础限制映射

	conn   BaseLimit // 连接基础限制
	stream BaseLimit // 流基础限制
}

// resourceLimitsMapFromBaseLimitMap 从基础限制映射创建资源限制映射
// 参数:
//   - baseLimitMap: map[K]BaseLimit - 基础限制映射,K 为可比较类型
//
// 返回值:
//   - map[K]ResourceLimits - 资源限制映射
func resourceLimitsMapFromBaseLimitMap[K comparable](baseLimitMap map[K]BaseLimit) map[K]ResourceLimits {
	// 如果基础限制映射为空,返回 nil
	if baseLimitMap == nil {
		return nil
	}

	// 创建输出映射
	out := make(map[K]ResourceLimits)
	// 遍历基础限制映射,将每个基础限制转换为资源限制
	for k, l := range baseLimitMap {
		out[k] = l.ToResourceLimits()
	}

	// 返回转换后的映射
	return out
}

// ToPartialLimitConfig 将 ConcreteLimitConfig 转换为 PartialLimitConfig。
// 返回的 PartialLimitConfig 不会有默认值。
// 返回值:
//   - PartialLimitConfig - 转换后的部分限制配置
func (cfg ConcreteLimitConfig) ToPartialLimitConfig() PartialLimitConfig {
	// 返回新的 PartialLimitConfig,将所有基础限制转换为资源限制
	return PartialLimitConfig{
		System:               cfg.system.ToResourceLimits(),                       // 转换系统限制
		Transient:            cfg.transient.ToResourceLimits(),                    // 转换临时限制
		AllowlistedSystem:    cfg.allowlistedSystem.ToResourceLimits(),            // 转换白名单系统限制
		AllowlistedTransient: cfg.allowlistedTransient.ToResourceLimits(),         // 转换白名单临时限制
		ServiceDefault:       cfg.serviceDefault.ToResourceLimits(),               // 转换服务默认限制
		Service:              resourceLimitsMapFromBaseLimitMap(cfg.service),      // 转换服务限制映射
		ServicePeerDefault:   cfg.servicePeerDefault.ToResourceLimits(),           // 转换服务对等节点默认限制
		ServicePeer:          resourceLimitsMapFromBaseLimitMap(cfg.servicePeer),  // 转换服务对等节点限制映射
		ProtocolDefault:      cfg.protocolDefault.ToResourceLimits(),              // 转换协议默认限制
		Protocol:             resourceLimitsMapFromBaseLimitMap(cfg.protocol),     // 转换协议限制映射
		ProtocolPeerDefault:  cfg.protocolPeerDefault.ToResourceLimits(),          // 转换协议对等节点默认限制
		ProtocolPeer:         resourceLimitsMapFromBaseLimitMap(cfg.protocolPeer), // 转换协议对等节点限制映射
		PeerDefault:          cfg.peerDefault.ToResourceLimits(),                  // 转换对等节点默认限制
		Peer:                 resourceLimitsMapFromBaseLimitMap(cfg.peer),         // 转换对等节点限制映射
		Conn:                 cfg.conn.ToResourceLimits(),                         // 转换连接限制
		Stream:               cfg.stream.ToResourceLimits(),                       // 转换流限制
	}
}

// Scale 扩展限制配置。
// 参数:
//   - memory: int64 - 允许堆栈消耗的内存量,对于专用节点建议使用系统内存的 1/8。如果内存小于 128 MB,将使用基本配置。
//   - numFD: int - 文件描述符数量
//
// 返回值:
//   - ConcreteLimitConfig - 扩展后的具体限制配置
func (cfg *ScalingLimitConfig) Scale(memory int64, numFD int) ConcreteLimitConfig {
	lc := ConcreteLimitConfig{
		system:               scale(cfg.SystemBaseLimit, cfg.SystemLimitIncrease, memory, numFD),                             // 扩展系统限制
		transient:            scale(cfg.TransientBaseLimit, cfg.TransientLimitIncrease, memory, numFD),                       // 扩展临时限制
		allowlistedSystem:    scale(cfg.AllowlistedSystemBaseLimit, cfg.AllowlistedSystemLimitIncrease, memory, numFD),       // 扩展白名单系统限制
		allowlistedTransient: scale(cfg.AllowlistedTransientBaseLimit, cfg.AllowlistedTransientLimitIncrease, memory, numFD), // 扩展白名单临时限制
		serviceDefault:       scale(cfg.ServiceBaseLimit, cfg.ServiceLimitIncrease, memory, numFD),                           // 扩展服务默认限制
		servicePeerDefault:   scale(cfg.ServicePeerBaseLimit, cfg.ServicePeerLimitIncrease, memory, numFD),                   // 扩展服务对等节点默认限制
		protocolDefault:      scale(cfg.ProtocolBaseLimit, cfg.ProtocolLimitIncrease, memory, numFD),                         // 扩展协议默认限制
		protocolPeerDefault:  scale(cfg.ProtocolPeerBaseLimit, cfg.ProtocolPeerLimitIncrease, memory, numFD),                 // 扩展协议对等节点默认限制
		peerDefault:          scale(cfg.PeerBaseLimit, cfg.PeerLimitIncrease, memory, numFD),                                 // 扩展对等节点默认限制
		conn:                 scale(cfg.ConnBaseLimit, cfg.ConnLimitIncrease, memory, numFD),                                 // 扩展连接限制
		stream:               scale(cfg.StreamBaseLimit, cfg.ConnLimitIncrease, memory, numFD),                               // 扩展流限制
	}

	// 如果存在服务限制配置,则扩展服务限制
	if cfg.ServiceLimits != nil {
		lc.service = make(map[string]BaseLimit) // 初始化服务限制映射
		for svc, l := range cfg.ServiceLimits {
			lc.service[svc] = scale(l.BaseLimit, l.BaseLimitIncrease, memory, numFD) // 扩展每个服务的限制
		}
	}

	// 如果存在协议限制配置,则扩展协议限制
	if cfg.ProtocolLimits != nil {
		lc.protocol = make(map[protocol.ID]BaseLimit) // 初始化协议限制映射
		for proto, l := range cfg.ProtocolLimits {
			lc.protocol[proto] = scale(l.BaseLimit, l.BaseLimitIncrease, memory, numFD) // 扩展每个协议的限制
		}
	}

	// 如果存在对等节点限制配置,则扩展对等节点限制
	if cfg.PeerLimits != nil {
		lc.peer = make(map[peer.ID]BaseLimit) // 初始化对等节点限制映射
		for p, l := range cfg.PeerLimits {
			lc.peer[p] = scale(l.BaseLimit, l.BaseLimitIncrease, memory, numFD) // 扩展每个对等节点的限制
		}
	}

	// 如果存在服务对等节点限制配置,则扩展服务对等节点限制
	if cfg.ServicePeerLimits != nil {
		lc.servicePeer = make(map[string]BaseLimit) // 初始化服务对等节点限制映射
		for svc, l := range cfg.ServicePeerLimits {
			lc.servicePeer[svc] = scale(l.BaseLimit, l.BaseLimitIncrease, memory, numFD) // 扩展每个服务对等节点的限制
		}
	}

	// 如果存在协议对等节点限制配置,则扩展协议对等节点限制
	if cfg.ProtocolPeerLimits != nil {
		lc.protocolPeer = make(map[protocol.ID]BaseLimit) // 初始化协议对等节点限制映射
		for p, l := range cfg.ProtocolPeerLimits {
			lc.protocolPeer[p] = scale(l.BaseLimit, l.BaseLimitIncrease, memory, numFD) // 扩展每个协议对等节点的限制
		}
	}
	return lc
}

// AutoScale 自动扩展限制配置
// 返回值:
//   - ConcreteLimitConfig - 自动扩展后的具体限制配置
func (cfg *ScalingLimitConfig) AutoScale() ConcreteLimitConfig {
	return cfg.Scale(
		int64(memory.TotalMemory())/8, // 使用系统总内存的 1/8
		getNumFDs()/2,                 // 使用可用文件描述符数量的一半
	)
}

// scale 根据内存和文件描述符数量扩展基础限制
// 参数:
//   - base: BaseLimit - 基础限制配置
//   - inc: BaseLimitIncrease - 限制增量配置
//   - memory: int64 - 可用内存大小
//   - numFD: int - 可用文件描述符数量
//
// 返回值:
//   - BaseLimit - 扩展后的基础限制
func scale(base BaseLimit, inc BaseLimitIncrease, memory int64, numFD int) BaseLimit {
	// mebibytesAvailable 表示可用的 MiB 数量,用于扩展限制
	// 如果小于 128MiB,设置为 0 以仅使用基本数量
	var mebibytesAvailable int
	if memory > 128<<20 {
		mebibytesAvailable = int((memory) >> 20) // 将字节转换为 MiB
	}

	// 创建扩展后的基础限制
	l := BaseLimit{
		StreamsInbound:  base.StreamsInbound + (inc.StreamsInbound*mebibytesAvailable)>>10,   // 扩展入站流限制
		StreamsOutbound: base.StreamsOutbound + (inc.StreamsOutbound*mebibytesAvailable)>>10, // 扩展出站流限制
		Streams:         base.Streams + (inc.Streams*mebibytesAvailable)>>10,                 // 扩展总流限制
		ConnsInbound:    base.ConnsInbound + (inc.ConnsInbound*mebibytesAvailable)>>10,       // 扩展入站连接限制
		ConnsOutbound:   base.ConnsOutbound + (inc.ConnsOutbound*mebibytesAvailable)>>10,     // 扩展出站连接限制
		Conns:           base.Conns + (inc.Conns*mebibytesAvailable)>>10,                     // 扩展总连接限制
		Memory:          base.Memory + (inc.Memory*int64(mebibytesAvailable))>>10,            // 扩展内存限制
		FD:              base.FD,                                                             // 设置文件描述符基础限制
	}

	// 如果指定了文件描述符比例且有可用的文件描述符
	if inc.FDFraction > 0 && numFD > 0 {
		l.FD = int(inc.FDFraction * float64(numFD)) // 根据比例计算文件描述符限制
		if l.FD < base.FD {
			l.FD = base.FD // 如果计算结果小于基础限制,则使用基础限制
		}
	}
	return l
}

// DefaultLimits 是默认限制器构造函数使用的限制。
// 参数:
//   - SystemBaseLimit: 系统基础限制配置
//   - SystemLimitIncrease: 系统限制增量配置
//   - TransientBaseLimit: 临时基础限制配置
//   - TransientLimitIncrease: 临时限制增量配置
//   - AllowlistedSystemBaseLimit: 白名单系统基础限制配置
//   - AllowlistedSystemLimitIncrease: 白名单系统限制增量配置
//   - AllowlistedTransientBaseLimit: 白名单临时基础限制配置
//   - AllowlistedTransientLimitIncrease: 白名单临时限制增量配置
//   - ServiceBaseLimit: 服务基础限制配置
//   - ServiceLimitIncrease: 服务限制增量配置
//   - ServicePeerBaseLimit: 服务对等节点基础限制配置
//   - ServicePeerLimitIncrease: 服务对等节点限制增量配置
//   - ProtocolBaseLimit: 协议基础限制配置
//   - ProtocolLimitIncrease: 协议限制增量配置
//   - ProtocolPeerBaseLimit: 协议对等节点基础限制配置
//   - ProtocolPeerLimitIncrease: 协议对等节点限制增量配置
//   - PeerBaseLimit: 对等节点基础限制配置
//   - PeerLimitIncrease: 对等节点限制增量配置
//   - ConnBaseLimit: 连接基础限制配置
//   - StreamBaseLimit: 流基础限制配置
//
// 返回值:
//   - ScalingLimitConfig: 包含所有限制配置的可扩展限制配置
var DefaultLimits = ScalingLimitConfig{
	SystemBaseLimit: BaseLimit{
		ConnsInbound:    64,        // 入站连接基础限制数量
		ConnsOutbound:   128,       // 出站连接基础限制数量
		Conns:           128,       // 总连接基础限制数量
		StreamsInbound:  64 * 16,   // 入站流基础限制数量
		StreamsOutbound: 128 * 16,  // 出站流基础限制数量
		Streams:         128 * 16,  // 总流基础限制数量
		Memory:          128 << 20, // 内存基础限制大小(128MB)
		FD:              256,       // 文件描述符基础限制数量
	},

	SystemLimitIncrease: BaseLimitIncrease{
		ConnsInbound:    64,       // 入站连接增量限制数量
		ConnsOutbound:   128,      // 出站连接增量限制数量
		Conns:           128,      // 总连接增量限制数量
		StreamsInbound:  64 * 16,  // 入站流增量限制数量
		StreamsOutbound: 128 * 16, // 出站流增量限制数量
		Streams:         128 * 16, // 总流增量限制数量
		Memory:          1 << 30,  // 内存增量限制大小(1GB)
		FDFraction:      1,        // 文件描述符增量限制比例
	},

	TransientBaseLimit: BaseLimit{
		ConnsInbound:    32,       // 临时入站连接基础限制数量
		ConnsOutbound:   64,       // 临时出站连接基础限制数量
		Conns:           64,       // 临时总连接基础限制数量
		StreamsInbound:  128,      // 临时入站流基础限制数量
		StreamsOutbound: 256,      // 临时出站流基础限制数量
		Streams:         256,      // 临时总流基础限制数量
		Memory:          32 << 20, // 临时内存基础限制大小(32MB)
		FD:              64,       // 临时文件描述符基础限制数量
	},

	TransientLimitIncrease: BaseLimitIncrease{
		ConnsInbound:    16,        // 临时入站连接增量限制数量
		ConnsOutbound:   32,        // 临时出站连接增量限制数量
		Conns:           32,        // 临时总连接增量限制数量
		StreamsInbound:  128,       // 临时入站流增量限制数量
		StreamsOutbound: 256,       // 临时出站流增量限制数量
		Streams:         256,       // 临时总流增量限制数量
		Memory:          128 << 20, // 临时内存增量限制大小(128MB)
		FDFraction:      0.25,      // 临时文件描述符增量限制比例
	},

	// 将白名单限制设置为与普通限制相同。白名单仅在达到正常的系统/临时限制时激活。
	// 因此,这些限制可以偏向于过大,因为大多数时候你甚至不会使用任何这些限制。
	// 如果你想针对白名单端点管理资源,请调低这些限制。
	AllowlistedSystemBaseLimit: BaseLimit{
		ConnsInbound:    64,        // 白名单系统入站连接基础限制数量
		ConnsOutbound:   128,       // 白名单系统出站连接基础限制数量
		Conns:           128,       // 白名单系统总连接基础限制数量
		StreamsInbound:  64 * 16,   // 白名单系统入站流基础限制数量
		StreamsOutbound: 128 * 16,  // 白名单系统出站流基础限制数量
		Streams:         128 * 16,  // 白名单系统总流基础限制数量
		Memory:          128 << 20, // 白名单系统内存基础限制大小(128MB)
		FD:              256,       // 白名单系统文件描述符基础限制数量
	},

	AllowlistedSystemLimitIncrease: BaseLimitIncrease{
		ConnsInbound:    64,       // 白名单系统入站连接增量限制数量
		ConnsOutbound:   128,      // 白名单系统出站连接增量限制数量
		Conns:           128,      // 白名单系统总连接增量限制数量
		StreamsInbound:  64 * 16,  // 白名单系统入站流增量限制数量
		StreamsOutbound: 128 * 16, // 白名单系统出站流增量限制数量
		Streams:         128 * 16, // 白名单系统总流增量限制数量
		Memory:          1 << 30,  // 白名单系统内存增量限制大小(1GB)
		FDFraction:      1,        // 白名单系统文件描述符增量限制比例
	},

	AllowlistedTransientBaseLimit: BaseLimit{
		ConnsInbound:    32,       // 白名单临时入站连接基础限制数量
		ConnsOutbound:   64,       // 白名单临时出站连接基础限制数量
		Conns:           64,       // 白名单临时总连接基础限制数量
		StreamsInbound:  128,      // 白名单临时入站流基础限制数量
		StreamsOutbound: 256,      // 白名单临时出站流基础限制数量
		Streams:         256,      // 白名单临时总流基础限制数量
		Memory:          32 << 20, // 白名单临时内存基础限制大小(32MB)
		FD:              64,       // 白名单临时文件描述符基础限制数量
	},

	AllowlistedTransientLimitIncrease: BaseLimitIncrease{
		ConnsInbound:    16,        // 白名单临时入站连接增量限制数量
		ConnsOutbound:   32,        // 白名单临时出站连接增量限制数量
		Conns:           32,        // 白名单临时总连接增量限制数量
		StreamsInbound:  128,       // 白名单临时入站流增量限制数量
		StreamsOutbound: 256,       // 白名单临时出站流增量限制数量
		Streams:         256,       // 白名单临时总流增量限制数量
		Memory:          128 << 20, // 白名单临时内存增量限制大小(128MB)
		FDFraction:      0.25,      // 白名单临时文件描述符增量限制比例
	},

	ServiceBaseLimit: BaseLimit{
		StreamsInbound:  1024,     // 服务入站流基础限制数量
		StreamsOutbound: 4096,     // 服务出站流基础限制数量
		Streams:         4096,     // 服务总流基础限制数量
		Memory:          64 << 20, // 服务内存基础限制大小(64MB)
	},

	ServiceLimitIncrease: BaseLimitIncrease{
		StreamsInbound:  512,       // 服务入站流增量限制数量
		StreamsOutbound: 2048,      // 服务出站流增量限制数量
		Streams:         2048,      // 服务总流增量限制数量
		Memory:          128 << 20, // 服务内存增量限制大小(128MB)
	},

	ServicePeerBaseLimit: BaseLimit{
		StreamsInbound:  128,      // 服务对等节点入站流基础限制数量
		StreamsOutbound: 256,      // 服务对等节点出站流基础限制数量
		Streams:         256,      // 服务对等节点总流基础限制数量
		Memory:          16 << 20, // 服务对等节点内存基础限制大小(16MB)
	},

	ServicePeerLimitIncrease: BaseLimitIncrease{
		StreamsInbound:  4,       // 服务对等节点入站流增量限制数量
		StreamsOutbound: 8,       // 服务对等节点出站流增量限制数量
		Streams:         8,       // 服务对等节点总流增量限制数量
		Memory:          4 << 20, // 服务对等节点内存增量限制大小(4MB)
	},

	ProtocolBaseLimit: BaseLimit{
		StreamsInbound:  512,      // 协议入站流基础限制数量
		StreamsOutbound: 2048,     // 协议出站流基础限制数量
		Streams:         2048,     // 协议总流基础限制数量
		Memory:          64 << 20, // 协议内存基础限制大小(64MB)
	},

	ProtocolLimitIncrease: BaseLimitIncrease{
		StreamsInbound:  256,       // 协议入站流增量限制数量
		StreamsOutbound: 512,       // 协议出站流增量限制数量
		Streams:         512,       // 协议总流增量限制数量
		Memory:          164 << 20, // 协议内存增量限制大小(164MB)
	},

	ProtocolPeerBaseLimit: BaseLimit{
		StreamsInbound:  64,       // 协议对等节点入站流基础限制数量
		StreamsOutbound: 128,      // 协议对等节点出站流基础限制数量
		Streams:         256,      // 协议对等节点总流基础限制数量
		Memory:          16 << 20, // 协议对等节点内存基础限制大小(16MB)
	},

	ProtocolPeerLimitIncrease: BaseLimitIncrease{
		StreamsInbound:  4,  // 协议对等节点入站流增量限制数量
		StreamsOutbound: 8,  // 协议对等节点出站流增量限制数量
		Streams:         16, // 协议对等节点总流增量限制数量
		Memory:          4,  // 协议对等节点内存增量限制大小(4B)
	},

	PeerBaseLimit: BaseLimit{
		// 目前设为 8,以匹配 swarm_dial.go 中可能进行的并发拨号数量。
		// 随着未来智能拨号工作的进行,我们应该降低这个值
		ConnsInbound:    8,        // 对等节点入站连接基础限制数量
		ConnsOutbound:   8,        // 对等节点出站连接基础限制数量
		Conns:           8,        // 对等节点总连接基础限制数量
		StreamsInbound:  256,      // 对等节点入站流基础限制数量
		StreamsOutbound: 512,      // 对等节点出站流基础限制数量
		Streams:         512,      // 对等节点总流基础限制数量
		Memory:          64 << 20, // 对等节点内存基础限制大小(64MB)
		FD:              4,        // 对等节点文件描述符基础限制数量
	},

	PeerLimitIncrease: BaseLimitIncrease{
		StreamsInbound:  128,       // 对等节点入站流增量限制数量
		StreamsOutbound: 256,       // 对等节点出站流增量限制数量
		Streams:         256,       // 对等节点总流增量限制数量
		Memory:          128 << 20, // 对等节点内存增量限制大小(128MB)
		FDFraction:      1.0 / 64,  // 对等节点文件描述符增量限制比例
	},

	ConnBaseLimit: BaseLimit{
		ConnsInbound:  1,        // 连接入站基础限制数量
		ConnsOutbound: 1,        // 连接出站基础限制数量
		Conns:         1,        // 连接总基础限制数量
		FD:            1,        // 连接文件描述符基础限制数量
		Memory:        32 << 20, // 连接内存基础限制大小(32MB)
	},

	StreamBaseLimit: BaseLimit{
		StreamsInbound:  1,        // 流入站基础限制数量
		StreamsOutbound: 1,        // 流出站基础限制数量
		Streams:         1,        // 流总基础限制数量
		Memory:          16 << 20, // 流内存基础限制大小(16MB)
	},
}

// infiniteBaseLimit 定义了一个无限制的基础限制配置
// 参数:
//   - Streams: int - 总流数量限制,设为最大整数值
//   - StreamsInbound: int - 入站流数量限制,设为最大整数值
//   - StreamsOutbound: int - 出站流数量限制,设为最大整数值
//   - Conns: int - 总连接数量限制,设为最大整数值
//   - ConnsInbound: int - 入站连接数量限制,设为最大整数值
//   - ConnsOutbound: int - 出站连接数量限制,设为最大整数值
//   - FD: int - 文件描述符数量限制,设为最大整数值
//   - Memory: int64 - 内存大小限制,设为最大64位整数值
var infiniteBaseLimit = BaseLimit{
	Streams:         math.MaxInt,   // 设置总流数量限制为最大整数值
	StreamsInbound:  math.MaxInt,   // 设置入站流数量限制为最大整数值
	StreamsOutbound: math.MaxInt,   // 设置出站流数量限制为最大整数值
	Conns:           math.MaxInt,   // 设置总连接数量限制为最大整数值
	ConnsInbound:    math.MaxInt,   // 设置入站连接数量限制为最大整数值
	ConnsOutbound:   math.MaxInt,   // 设置出站连接数量限制为最大整数值
	FD:              math.MaxInt,   // 设置文件描述符数量限制为最大整数值
	Memory:          math.MaxInt64, // 设置内存大小限制为最大64位整数值
}

// InfiniteLimits 是一个使用无限限制的限制器配置,因此实际上不限制任何内容。
// 请记住,操作系统会限制应用程序可以使用的文件描述符数量。
// 参数:
//   - system: BaseLimit - 系统级别的无限制配置
//   - transient: BaseLimit - 临时的无限制配置
//   - allowlistedSystem: BaseLimit - 白名单系统的无限制配置
//   - allowlistedTransient: BaseLimit - 白名单临时的无限制配置
//   - serviceDefault: BaseLimit - 服务默认的无限制配置
//   - servicePeerDefault: BaseLimit - 服务对等节点默认的无限制配置
//   - protocolDefault: BaseLimit - 协议默认的无限制配置
//   - protocolPeerDefault: BaseLimit - 协议对等节点默认的无限制配置
//   - peerDefault: BaseLimit - 对等节点默认的无限制配置
//   - conn: BaseLimit - 连接的无限制配置
//   - stream: BaseLimit - 流的无限制配置
var InfiniteLimits = ConcreteLimitConfig{
	system:               infiniteBaseLimit, // 设置系统级别的无限制配置
	transient:            infiniteBaseLimit, // 设置临时的无限制配置
	allowlistedSystem:    infiniteBaseLimit, // 设置白名单系统的无限制配置
	allowlistedTransient: infiniteBaseLimit, // 设置白名单临时的无限制配置
	serviceDefault:       infiniteBaseLimit, // 设置服务默认的无限制配置
	servicePeerDefault:   infiniteBaseLimit, // 设置服务对等节点默认的无限制配置
	protocolDefault:      infiniteBaseLimit, // 设置协议默认的无限制配置
	protocolPeerDefault:  infiniteBaseLimit, // 设置协议对等节点默认的无限制配置
	peerDefault:          infiniteBaseLimit, // 设置对等节点默认的无限制配置
	conn:                 infiniteBaseLimit, // 设置连接的无限制配置
	stream:               infiniteBaseLimit, // 设置流的无限制配置
}
