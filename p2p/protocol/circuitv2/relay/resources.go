package relay

import (
	"time"
)

// Resources 表示与中继服务相关的资源限制
type Resources struct {
	// Limit 是可选的中继连接限制
	Limit *RelayLimit

	// ReservationTTL 是新预约(或刷新预约)的持续时间
	// 默认为1小时
	ReservationTTL time.Duration

	// MaxReservations 是活跃中继槽位的最大数量;默认为128
	MaxReservations int
	// MaxCircuits 是每个节点的最大开放中继连接数;默认为16
	MaxCircuits int
	// BufferSize 是中继连接缓冲区的大小;默认为2048
	BufferSize int

	// MaxReservationsPerPeer 是来自同一节点的最大预约数;默认为4
	//
	// 已弃用:每个节点只需要1个预约
	MaxReservationsPerPeer int
	// MaxReservationsPerIP 是来自同一IP地址的最大预约数;默认为8
	MaxReservationsPerIP int
	// MaxReservationsPerASN 是来自同一ASN的最大预约数;默认为32
	MaxReservationsPerASN int
}

// RelayLimit 表示每个中继连接的资源限制
type RelayLimit struct {
	// Duration 是重置中继连接前的时间限制;默认为2分钟
	Duration time.Duration
	// Data 是重置连接前中继的数据限制(每个方向)
	// 默认为128KB
	Data int64
}

// DefaultResources 返回填充了默认值的Resources对象
// 返回值:
//   - Resources 默认的资源限制配置
func DefaultResources() Resources {
	return Resources{
		// 使用默认限制
		Limit: DefaultLimit(),

		// 预约有效期为1小时
		ReservationTTL: time.Hour,

		// 最大预约数为128
		MaxReservations: 128,
		// 每个节点最大16个中继电路
		MaxCircuits: 16,
		// 缓冲区大小为2048字节
		BufferSize: 2048,

		// 每个节点最多1个预约
		MaxReservationsPerPeer: 1,
		// 每个IP最多8个预约
		MaxReservationsPerIP: 8,
		// 每个ASN最多32个预约
		MaxReservationsPerASN: 32,
	}
}

// DefaultLimit 返回填充了默认值的RelayLimit对象
// 返回值:
//   - *RelayLimit 默认的中继限制配置
func DefaultLimit() *RelayLimit {
	return &RelayLimit{
		// 默认持续时间为2分钟
		Duration: 2 * time.Minute,
		// 默认数据限制为128KB
		Data: 1 << 17, // 128K
	}
}
