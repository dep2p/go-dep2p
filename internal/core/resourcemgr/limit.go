package resourcemgr

import pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"

// DefaultLimitConfig 返回默认的资源限制配置
func DefaultLimitConfig() *pkgif.LimitConfig {
	return &pkgif.LimitConfig{
		// 系统级限制
		System: pkgif.Limit{
			Streams:         10000,       // 最大 10000 个流
			StreamsInbound:  5000,        // 最大 5000 个入站流
			StreamsOutbound: 5000,        // 最大 5000 个出站流
			Conns:           1000,        // 最大 1000 个连接
			ConnsInbound:    800,         // 最大 800 个入站连接
			ConnsOutbound:   200,         // 最大 200 个出站连接
			FD:              900,         // 最大 900 个文件描述符
			Memory:          1 << 30,     // 1GB 内存
		},

		// 临时资源限制（握手期间）
		Transient: pkgif.Limit{
			Streams:         500,     // 最大 500 个临时流
			StreamsInbound:  250,     // 最大 250 个入站临时流
			StreamsOutbound: 250,     // 最大 250 个出站临时流
			Conns:           100,     // 最大 100 个临时连接
			ConnsInbound:    50,      // 最大 50 个入站临时连接
			ConnsOutbound:   50,      // 最大 50 个出站临时连接
			FD:              100,     // 最大 100 个临时文件描述符
			Memory:          1 << 28, // 256MB 临时内存
		},

		// 服务默认限制
		ServiceDefault: pkgif.Limit{
			Streams:         200,     // 每个服务最大 200 个流
			StreamsInbound:  100,     // 每个服务最大 100 个入站流
			StreamsOutbound: 100,     // 每个服务最大 100 个出站流
			Memory:          1 << 27, // 每个服务 128MB 内存
		},

		// 服务-节点默认限制
		ServicePeerDefault: pkgif.Limit{
			Streams:         20,      // 每个服务-节点最大 20 个流
			StreamsInbound:  10,      // 每个服务-节点最大 10 个入站流
			StreamsOutbound: 10,      // 每个服务-节点最大 10 个出站流
			Memory:          1 << 24, // 每个服务-节点 16MB 内存
		},

		// 协议默认限制
		ProtocolDefault: pkgif.Limit{
			Streams:         200,     // 每个协议最大 200 个流
			StreamsInbound:  100,     // 每个协议最大 100 个入站流
			StreamsOutbound: 100,     // 每个协议最大 100 个出站流
			Memory:          1 << 27, // 每个协议 128MB 内存
		},

		// 协议-节点默认限制
		ProtocolPeerDefault: pkgif.Limit{
			Streams:         20,      // 每个协议-节点最大 20 个流
			StreamsInbound:  10,      // 每个协议-节点最大 10 个入站流
			StreamsOutbound: 10,      // 每个协议-节点最大 10 个出站流
			Memory:          1 << 24, // 每个协议-节点 16MB 内存
		},

		// 节点默认限制
		PeerDefault: pkgif.Limit{
			Streams:         100,     // 每个节点最大 100 个流
			StreamsInbound:  50,      // 每个节点最大 50 个入站流
			StreamsOutbound: 50,      // 每个节点最大 50 个出站流
			Conns:           10,      // 每个节点最大 10 个连接
			ConnsInbound:    5,       // 每个节点最大 5 个入站连接
			ConnsOutbound:   5,       // 每个节点最大 5 个出站连接
			FD:              10,      // 每个节点最大 10 个文件描述符
			Memory:          1 << 26, // 每个节点 64MB 内存
		},

		// 连接限制
		Conn: pkgif.Limit{
			Streams:         50,      // 每个连接最大 50 个流
			StreamsInbound:  25,      // 每个连接最大 25 个入站流
			StreamsOutbound: 25,      // 每个连接最大 25 个出站流
			FD:              1,       // 每个连接 1 个文件描述符
			Memory:          1 << 25, // 每个连接 32MB 内存
		},

		// 流限制
		Stream: pkgif.Limit{
			Memory: 1 << 24, // 每个流 16MB 内存
		},
	}
}

// checkLimit 检查当前值是否超过限制
// current: 当前值
// limit: 限制值（0 表示无限制）
// 返回 ErrResourceLimitExceeded 如果超出限制
func checkLimit(current, limit int) error {
	if limit > 0 && current > limit {
		return ErrResourceLimitExceeded
	}
	return nil
}

// checkMemoryLimit 检查内存限制（带优先级）
// current: 当前内存使用量
// toReserve: 要预留的内存量
// limit: 内存限制
// prio: 优先级（0-255）
//
// 预留成功当: current + toReserve <= limit * (prio+1) / 256
// 例如：prio=255 时，threshold=limit, 可以使用全部资源
//       prio=101 时，threshold=limit*102/256≈40%
func checkMemoryLimit(current, toReserve, limit int64, prio uint8) error {
	if limit <= 0 {
		return nil // 无限制
	}

	// 计算新的使用量
	newUsage := current + toReserve
	if newUsage < 0 {
		// 溢出检查
		return ErrResourceLimitExceeded
	}

	// 计算阈值：limit * (prio+1) / 256
	// 特殊处理：prio=255 时直接比较 limit
	var threshold int64
	if prio == pkgif.ReservationPriorityAlways {
		threshold = limit
	} else {
		threshold = (limit * int64(prio+1)) / 256
	}

	if newUsage > threshold {
		return ErrResourceLimitExceeded
	}

	return nil
}
