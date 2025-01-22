/*
rcmgr 包是 go-dep2p 的资源管理器。
它允许你跟踪整个 go-dep2p 进程中使用的资源。
同时确保进程不会使用超过你定义的限制的资源。
资源管理器只知道它被告知的内容，因此这个库的使用者(无论是 go-dep2p 还是 go-dep2p 的用户)有责任在实际分配资源之前与资源管理器确认。
*/
package rcmgr

import (
	"encoding/json"
	"io"
	"math"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// Limit 是一个指定基本资源限制的接口。
type Limit interface {
	// GetMemoryLimit 返回(当前的)内存限制。
	// 返回值:
	//   - int64 - 内存限制大小(字节)
	GetMemoryLimit() int64

	// GetStreamLimit 返回入站或出站流的限制。
	// 参数:
	//   - dir: network.Direction - 流的方向(入站/出站)
	// 返回值:
	//   - int - 流的数量限制
	GetStreamLimit(network.Direction) int

	// GetStreamTotalLimit 返回总流限制
	// 返回值:
	//   - int - 总流数量限制
	GetStreamTotalLimit() int

	// GetConnLimit 返回入站或出站连接的限制。
	// 参数:
	//   - dir: network.Direction - 连接的方向(入站/出站)
	// 返回值:
	//   - int - 连接数量限制
	GetConnLimit(network.Direction) int

	// GetConnTotalLimit 返回总连接限制
	// 返回值:
	//   - int - 总连接数量限制
	GetConnTotalLimit() int

	// GetFDLimit 返回文件描述符限制。
	// 返回值:
	//   - int - 文件描述符数量限制
	GetFDLimit() int
}

// Limiter 是为资源管理器提供限制的接口。
type Limiter interface {
	// GetSystemLimits 返回系统级别的资源限制
	// 返回值:
	//   - Limit - 系统资源限制
	GetSystemLimits() Limit

	// GetTransientLimits 返回临时资源限制
	// 返回值:
	//   - Limit - 临时资源限制
	GetTransientLimits() Limit

	// GetAllowlistedSystemLimits 返回白名单系统资源限制
	// 返回值:
	//   - Limit - 白名单系统资源限制
	GetAllowlistedSystemLimits() Limit

	// GetAllowlistedTransientLimits 返回白名单临时资源限制
	// 返回值:
	//   - Limit - 白名单临时资源限制
	GetAllowlistedTransientLimits() Limit

	// GetServiceLimits 返回指定服务的资源限制
	// 参数:
	//   - svc: string - 服务名称
	// 返回值:
	//   - Limit - 服务资源限制
	GetServiceLimits(svc string) Limit

	// GetServicePeerLimits 返回指定服务的对等节点资源限制
	// 参数:
	//   - svc: string - 服务名称
	// 返回值:
	//   - Limit - 服务对等节点资源限制
	GetServicePeerLimits(svc string) Limit

	// GetProtocolLimits 返回指定协议的资源限制
	// 参数:
	//   - proto: protocol.ID - 协议ID
	// 返回值:
	//   - Limit - 协议资源限制
	GetProtocolLimits(proto protocol.ID) Limit

	// GetProtocolPeerLimits 返回指定协议的对等节点资源限制
	// 参数:
	//   - proto: protocol.ID - 协议ID
	// 返回值:
	//   - Limit - 协议对等节点资源限制
	GetProtocolPeerLimits(proto protocol.ID) Limit

	// GetPeerLimits 返回指定对等节点的资源限制
	// 参数:
	//   - p: peer.ID - 对等节点ID
	// 返回值:
	//   - Limit - 对等节点资源限制
	GetPeerLimits(p peer.ID) Limit

	// GetStreamLimits 返回指定对等节点的流资源限制
	// 参数:
	//   - p: peer.ID - 对等节点ID
	// 返回值:
	//   - Limit - 流资源限制
	GetStreamLimits(p peer.ID) Limit

	// GetConnLimits 返回连接资源限制
	// 返回值:
	//   - Limit - 连接资源限制
	GetConnLimits() Limit
}

// NewDefaultLimiterFromJSON 通过解析 JSON 配置创建一个新的限制器，使用默认限制作为回退。
// 参数:
//   - in: io.Reader - JSON 配置输入流
//
// 返回值:
//   - Limiter - 创建的限制器
//   - error 错误信息
func NewDefaultLimiterFromJSON(in io.Reader) (Limiter, error) {
	return NewLimiterFromJSON(in, DefaultLimits.AutoScale()) // 使用自动扩展的默认限制作为回退
}

// NewLimiterFromJSON 通过解析 JSON 配置创建一个新的限制器。
// 参数:
//   - in: io.Reader - JSON 配置输入流
//   - defaults: ConcreteLimitConfig - 默认限制配置
//
// 返回值:
//   - Limiter - 创建的限制器
//   - error 错误信息
func NewLimiterFromJSON(in io.Reader, defaults ConcreteLimitConfig) (Limiter, error) {
	cfg, err := readLimiterConfigFromJSON(in, defaults) // 读取并解析 JSON 配置
	if err != nil {
		log.Debugf("读取并解析 JSON 配置失败: %v", err)
		return nil, err
	}
	return &fixedLimiter{cfg}, nil // 返回固定限制器
}

// readLimiterConfigFromJSON 从 JSON 读取限制器配置
// 参数:
//   - in: io.Reader - JSON 配置输入流
//   - defaults: ConcreteLimitConfig - 默认限制配置
//
// 返回值:
//   - ConcreteLimitConfig - 解析后的具体限制配置
//   - error 错误信息
func readLimiterConfigFromJSON(in io.Reader, defaults ConcreteLimitConfig) (ConcreteLimitConfig, error) {
	var cfg PartialLimitConfig                               // 声明部分限制配置
	if err := json.NewDecoder(in).Decode(&cfg); err != nil { // 解码 JSON 到配置
		log.Debugf("解码 JSON 到配置失败: %v", err)
		return ConcreteLimitConfig{}, err
	}
	return cfg.Build(defaults), nil // 使用默认值构建完整配置
}

// fixedLimiter 是一个具有固定限制的限制器。
type fixedLimiter struct {
	ConcreteLimitConfig // 嵌入具体限制配置
}

var _ Limiter = (*fixedLimiter)(nil) // 确保 fixedLimiter 实现了 Limiter 接口

// NewFixedLimiter 创建一个新的固定限制器
// 参数:
//   - conf: ConcreteLimitConfig - 具体限制配置
//
// 返回值:
//   - Limiter - 创建的限制器
func NewFixedLimiter(conf ConcreteLimitConfig) Limiter {
	log.Debugf("使用配置初始化新的限制器: %v", conf) // 记录调试日志
	return &fixedLimiter{conf}           // 返回固定限制器
}

// BaseLimit 是基本资源限制的混入类型。
type BaseLimit struct {
	Streams         int   `json:",omitempty"` // 总流数量限制
	StreamsInbound  int   `json:",omitempty"` // 入站流数量限制
	StreamsOutbound int   `json:",omitempty"` // 出站流数量限制
	Conns           int   `json:",omitempty"` // 总连接数量限制
	ConnsInbound    int   `json:",omitempty"` // 入站连接数量限制
	ConnsOutbound   int   `json:",omitempty"` // 出站连接数量限制
	FD              int   `json:",omitempty"` // 文件描述符数量限制
	Memory          int64 `json:",omitempty"` // 内存大小限制(字节)
}

// valueOrBlockAll 将整数值转换为限制值
// 参数:
//   - n: int - 输入的整数值
//
// 返回值:
//   - LimitVal - 转换后的限制值
func valueOrBlockAll(n int) LimitVal {
	if n == 0 { // 如果值为 0
		return BlockAllLimit // 返回阻止所有限制
	} else if n == math.MaxInt { // 如果值为最大整数
		return Unlimited // 返回无限制
	}
	return LimitVal(n) // 返回数值限制
}

// valueOrBlockAll64 将 64 位整数值转换为限制值
// 参数:
//   - n: int64 - 输入的 64 位整数值
//
// 返回值:
//   - LimitVal64 - 转换后的 64 位限制值
func valueOrBlockAll64(n int64) LimitVal64 {
	if n == 0 { // 如果值为 0
		return BlockAllLimit64 // 返回阻止所有限制
	} else if n == math.MaxInt { // 如果值为最大整数
		return Unlimited64 // 返回无限制
	}
	return LimitVal64(n) // 返回数值限制
}

// ToResourceLimits 将 BaseLimit 转换为 ResourceLimits
// 返回值:
//   - ResourceLimits - 转换后的资源限制
func (l BaseLimit) ToResourceLimits() ResourceLimits {
	return ResourceLimits{
		Streams:         valueOrBlockAll(l.Streams),         // 转换总流限制
		StreamsInbound:  valueOrBlockAll(l.StreamsInbound),  // 转换入站流限制
		StreamsOutbound: valueOrBlockAll(l.StreamsOutbound), // 转换出站流限制
		Conns:           valueOrBlockAll(l.Conns),           // 转换总连接限制
		ConnsInbound:    valueOrBlockAll(l.ConnsInbound),    // 转换入站连接限制
		ConnsOutbound:   valueOrBlockAll(l.ConnsOutbound),   // 转换出站连接限制
		FD:              valueOrBlockAll(l.FD),              // 转换文件描述符限制
		Memory:          valueOrBlockAll64(l.Memory),        // 转换内存限制
	}
}

// Apply 用 l2 的值覆盖所有零值限制
// 参数:
//   - l2: BaseLimit - 源限制配置
func (l *BaseLimit) Apply(l2 BaseLimit) {
	if l.Streams == 0 { // 如果总流限制为 0
		l.Streams = l2.Streams // 使用源限制值
	}
	if l.StreamsInbound == 0 { // 如果入站流限制为 0
		l.StreamsInbound = l2.StreamsInbound // 使用源限制值
	}
	if l.StreamsOutbound == 0 { // 如果出站流限制为 0
		l.StreamsOutbound = l2.StreamsOutbound // 使用源限制值
	}
	if l.Conns == 0 { // 如果总连接限制为 0
		l.Conns = l2.Conns // 使用源限制值
	}
	if l.ConnsInbound == 0 { // 如果入站连接限制为 0
		l.ConnsInbound = l2.ConnsInbound // 使用源限制值
	}
	if l.ConnsOutbound == 0 { // 如果出站连接限制为 0
		l.ConnsOutbound = l2.ConnsOutbound // 使用源限制值
	}
	if l.Memory == 0 { // 如果内存限制为 0
		l.Memory = l2.Memory // 使用源限制值
	}
	if l.FD == 0 { // 如果文件描述符限制为 0
		l.FD = l2.FD // 使用源限制值
	}
}

// BaseLimitIncrease 是每 GiB 允许内存的增加量。
type BaseLimitIncrease struct {
	Streams         int     `json:",omitempty"` // 每 GiB 内存增加的总流数量
	StreamsInbound  int     `json:",omitempty"` // 每 GiB 内存增加的入站流数量
	StreamsOutbound int     `json:",omitempty"` // 每 GiB 内存增加的出站流数量
	Conns           int     `json:",omitempty"` // 每 GiB 内存增加的总连接数量
	ConnsInbound    int     `json:",omitempty"` // 每 GiB 内存增加的入站连接数量
	ConnsOutbound   int     `json:",omitempty"` // 每 GiB 内存增加的出站连接数量
	Memory          int64   `json:",omitempty"` // 每 GiB 内存增加的内存大小(字节)
	FDFraction      float64 `json:",omitempty"` // 文件描述符增加的比例(0-1)
}

// Apply 用 l2 的值覆盖所有零值限制
// 参数:
//   - l2: BaseLimitIncrease - 源限制增量配置
func (l *BaseLimitIncrease) Apply(l2 BaseLimitIncrease) {
	if l.Streams == 0 { // 如果总流增量为 0
		l.Streams = l2.Streams // 使用源增量值
	}
	if l.StreamsInbound == 0 { // 如果入站流增量为 0
		l.StreamsInbound = l2.StreamsInbound // 使用源增量值
	}
	if l.StreamsOutbound == 0 { // 如果出站流增量为 0
		l.StreamsOutbound = l2.StreamsOutbound // 使用源增量值
	}
	if l.Conns == 0 { // 如果总连接增量为 0
		l.Conns = l2.Conns // 使用源增量值
	}
	if l.ConnsInbound == 0 { // 如果入站连接增量为 0
		l.ConnsInbound = l2.ConnsInbound // 使用源增量值
	}
	if l.ConnsOutbound == 0 { // 如果出站连接增量为 0
		l.ConnsOutbound = l2.ConnsOutbound // 使用源增量值
	}
	if l.Memory == 0 { // 如果内存增量为 0
		l.Memory = l2.Memory // 使用源增量值
	}
	if l.FDFraction == 0 { // 如果文件描述符比例为 0
		l.FDFraction = l2.FDFraction // 使用源比例值
	}
}

// GetStreamLimit 获取指定方向的流限制
// 参数:
//   - dir: network.Direction - 流的方向(入站/出站)
//
// 返回值:
//   - int - 流的数量限制
func (l BaseLimit) GetStreamLimit(dir network.Direction) int {
	if dir == network.DirInbound { // 如果是入站方向
		return l.StreamsInbound // 返回入站流限制
	} else {
		return l.StreamsOutbound // 返回出站流限制
	}
}

// GetStreamTotalLimit 获取总流限制
// 返回值:
//   - int - 总流数量限制
func (l BaseLimit) GetStreamTotalLimit() int {
	return l.Streams // 返回总流限制
}

// GetConnLimit 获取指定方向的连接限制
// 参数:
//   - dir: network.Direction - 连接的方向(入站/出站)
//
// 返回值:
//   - int - 连接数量限制
func (l BaseLimit) GetConnLimit(dir network.Direction) int {
	if dir == network.DirInbound { // 如果是入站方向
		return l.ConnsInbound // 返回入站连接限制
	} else {
		return l.ConnsOutbound // 返回出站连接限制
	}
}

// GetConnTotalLimit 获取总连接限制
// 返回值:
//   - int - 总连接数量限制
func (l BaseLimit) GetConnTotalLimit() int {
	return l.Conns // 返回总连接限制
}

// GetFDLimit 获取文件描述符限制
// 返回值:
//   - int - 文件描述符数量限制
func (l BaseLimit) GetFDLimit() int {
	return l.FD // 返回文件描述符限制
}

// GetMemoryLimit 获取内存限制
// 返回值:
//   - int64 - 内存大小限制(字节)
func (l BaseLimit) GetMemoryLimit() int64 {
	return l.Memory // 返回内存限制
}

// GetSystemLimits 获取系统限制
// 返回值:
//   - Limit - 系统资源限制
func (l *fixedLimiter) GetSystemLimits() Limit {
	return &l.system // 返回系统限制
}

// GetTransientLimits 获取临时限制
// 返回值:
//   - Limit - 临时资源限制
func (l *fixedLimiter) GetTransientLimits() Limit {
	return &l.transient // 返回临时限制
}

// GetAllowlistedSystemLimits 获取白名单系统限制
// 返回值:
//   - Limit - 白名单系统资源限制
func (l *fixedLimiter) GetAllowlistedSystemLimits() Limit {
	return &l.allowlistedSystem // 返回白名单系统限制
}

// GetAllowlistedTransientLimits 获取白名单临时限制
// 返回值:
//   - Limit - 白名单临时资源限制
func (l *fixedLimiter) GetAllowlistedTransientLimits() Limit {
	return &l.allowlistedTransient // 返回白名单临时限制
}

// GetServiceLimits 获取指定服务的限制
// 参数:
//   - svc: string - 服务名称
//
// 返回值:
//   - Limit - 服务资源限制
func (l *fixedLimiter) GetServiceLimits(svc string) Limit {
	sl, ok := l.service[svc] // 获取服务限制
	if !ok {                 // 如果服务限制不存在
		return &l.serviceDefault // 返回服务默认限制
	}
	return &sl // 返回服务限制
}

// GetServicePeerLimits 获取指定服务的对等节点限制
// 参数:
//   - svc: string - 服务名称
//
// 返回值:
//   - Limit - 服务对等节点资源限制
func (l *fixedLimiter) GetServicePeerLimits(svc string) Limit {
	pl, ok := l.servicePeer[svc] // 获取服务对等节点限制
	if !ok {                     // 如果服务对等节点限制不存在
		return &l.servicePeerDefault // 返回服务对等节点默认限制
	}
	return &pl // 返回服务对等节点限制
}

// GetProtocolLimits 获取指定协议的限制
// 参数:
//   - proto: protocol.ID - 协议ID
//
// 返回值:
//   - Limit - 协议资源限制
func (l *fixedLimiter) GetProtocolLimits(proto protocol.ID) Limit {
	pl, ok := l.protocol[proto] // 获取协议限制
	if !ok {                    // 如果协议限制不存在
		return &l.protocolDefault // 返回协议默认限制
	}
	return &pl // 返回协议限制
}

// GetProtocolPeerLimits 获取指定协议的对等节点限制
// 参数:
//   - proto: protocol.ID - 协议ID
//
// 返回值:
//   - Limit - 协议对等节点资源限制
func (l *fixedLimiter) GetProtocolPeerLimits(proto protocol.ID) Limit {
	pl, ok := l.protocolPeer[proto] // 获取协议对等节点限制
	if !ok {                        // 如果协议对等节点限制不存在
		return &l.protocolPeerDefault // 返回协议对等节点默认限制
	}
	return &pl // 返回协议对等节点限制
}

// GetPeerLimits 获取指定对等节点的限制
// 参数:
//   - p: peer.ID - 对等节点ID
//
// 返回值:
//   - Limit - 对等节点资源限制
func (l *fixedLimiter) GetPeerLimits(p peer.ID) Limit {
	pl, ok := l.peer[p] // 获取对等节点限制
	if !ok {            // 如果对等节点限制不存在
		return &l.peerDefault // 返回对等节点默认限制
	}
	return &pl // 返回对等节点限制
}

// GetStreamLimits 获取指定对等节点的流限制
// 参数:
//   - p: peer.ID - 对等节点ID
//
// 返回值:
//   - Limit - 流资源限制
func (l *fixedLimiter) GetStreamLimits(_ peer.ID) Limit {
	return &l.stream // 返回流限制
}

// GetConnLimits 获取连接限制
// 返回值:
//   - Limit - 连接资源限制
func (l *fixedLimiter) GetConnLimits() Limit {
	return &l.conn // 返回连接限制
}
