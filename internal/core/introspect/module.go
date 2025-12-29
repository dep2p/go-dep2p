// Package introspect 提供本地自省 HTTP 服务
package introspect

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
)

// ModuleInput 模块输入
type ModuleInput struct {
	fx.In

	Endpoint endpoint.Endpoint
	Realm    realmif.RealmManager `optional:"true"`
	Relay    relayif.RelayServer  `optional:"true"`
}

// ModuleOutput 模块输出
type ModuleOutput struct {
	fx.Out

	Server *Server
}

// ProvideServer 提供自省服务
func ProvideServer(in ModuleInput) ModuleOutput {
	cfg := Config{
		Endpoint: in.Endpoint,
		Realm:    in.Realm,
		Relay:    in.Relay,
	}
	return ModuleOutput{
		Server: New(cfg),
	}
}

// Module 返回 introspect fx 模块
func Module() fx.Option {
	return fx.Module("introspect",
		fx.Provide(ProvideServer),
		fx.Invoke(func(lc fx.Lifecycle, s *Server) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					return s.Start(ctx)
				},
				OnStop: func(ctx context.Context) error {
					return s.Stop()
				},
			})
		}),
	)
}

