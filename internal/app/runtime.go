package app

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces/address"
	"github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/interfaces/liveness"
	"github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	"github.com/dep2p/go-dep2p/pkg/interfaces/realm"
)

// Runtime 表示一个已通过 fx 组装完成的 dep2p 运行时。
//
// 注意：
// - endpoint.Endpoint 保持最小稳定接口，不直接聚合 Messaging/ConnMgr/Liveness/Realm
// - pkg/dep2p Facade（Node）会组合 Runtime 暴露"一把梭"的用户体验 API
type Runtime struct {
	Endpoint          endpoint.Endpoint
	ConnectionManager connmgr.ConnectionManager
	Messaging         messaging.MessagingService
	Liveness          liveness.LivenessService
	Realm             realm.RealmManager
	AddressParser     address.AddressParser // 地址解析器（通过 Fx 注入）
	Reachability      reachabilityif.Coordinator

	stop func(ctx context.Context) error
}

// Stop 停止运行时（触发 fx 生命周期 OnStop）。
func (r *Runtime) Stop(ctx context.Context) error {
	if r.stop == nil {
		return nil
	}
	return r.stop(ctx)
}
