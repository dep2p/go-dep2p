package event

import (
	"github.com/dep2p/go-dep2p/core/network"
)

// EvtLocalReachabilityChanged 是一个事件结构体,当本地节点的可达性状态发生变化时会发出此事件。
//
// 此事件通常由 AutoNAT 子系统发出。
type EvtLocalReachabilityChanged struct {
	Reachability network.Reachability
}
