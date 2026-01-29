package peerstore

import (
	"math"
	"time"
)

// 地址 TTL 常量
const (
	// PermanentAddrTTL 永久地址（如引导节点）
	PermanentAddrTTL = time.Duration(math.MaxInt64 - 1)

	// ConnectedAddrTTL 连接成功的地址（30 分钟）
	ConnectedAddrTTL = 30 * time.Minute

	// RecentlyConnectedAddrTTL 最近连接的地址（15 分钟）
	RecentlyConnectedAddrTTL = 15 * time.Minute

	// DiscoveredAddrTTL DHT/Rendezvous 发现的地址（10 分钟）
	DiscoveredAddrTTL = 10 * time.Minute

	// LocalAddrTTL mDNS 发现的地址（5 分钟）
	LocalAddrTTL = 5 * time.Minute

	// TempAddrTTL 临时地址（2 分钟）
	TempAddrTTL = 2 * time.Minute
)
