// Package realm 提供 Protocol 服务工厂方法
package realm

import (
	"context"
	"fmt"

	"github.com/dep2p/go-dep2p/internal/protocol/liveness"
	"github.com/dep2p/go-dep2p/internal/protocol/messaging"
	"github.com/dep2p/go-dep2p/internal/protocol/pubsub"
	"github.com/dep2p/go-dep2p/internal/protocol/streams"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              Protocol 服务工厂
// ============================================================================

// createProtocolServices 为 Realm 创建所有 Protocol 服务
//
// 这些服务绑定到特定的 Realm，协议 ID 包含 RealmID
func (m *Manager) createProtocolServices(_ context.Context, realm *realmImpl) error {
	if m.host == nil {
		return fmt.Errorf("host is required for protocol services")
	}

	// 1. 创建 Messaging 服务
	msgSvc, err := m.createMessagingService(realm)
	if err != nil {
		return fmt.Errorf("failed to create messaging service: %w", err)
	}
	realm.messaging = msgSvc

	// 2. 创建 PubSub 服务
	pubsubSvc, err := m.createPubSubService(realm)
	if err != nil {
		return fmt.Errorf("failed to create pubsub service: %w", err)
	}
	realm.pubsub = pubsubSvc

	// 3. 创建 Streams 服务
	streamsSvc, err := m.createStreamsService(realm)
	if err != nil {
		return fmt.Errorf("failed to create streams service: %w", err)
	}
	realm.streams = streamsSvc

	// 4. 创建 Liveness 服务
	livenessSvc, err := m.createLivenessService(realm)
	if err != nil {
		return fmt.Errorf("failed to create liveness service: %w", err)
	}
	realm.liveness = livenessSvc

	//
	// 用于检测 QUIC 直连的健康状态，加速离线检测
	if swarm := m.host.Network(); swarm != nil {
		if setter, ok := swarm.(interface{ SetLiveness(pkgif.Liveness) }); ok {
			setter.SetLiveness(livenessSvc)
			logger.Debug("Liveness 已注入到 Swarm", "realmID", realm.id)
		}
	}

	return nil
}

// createMessagingService 创建绑定到 Realm 的 Messaging 服务
func (m *Manager) createMessagingService(realm *realmImpl) (pkgif.Messaging, error) {
	// 使用 NewForRealm 创建绑定到特定 Realm 的服务
	return messaging.NewForRealm(m.host, realm)
}

// createPubSubService 创建绑定到 Realm 的 PubSub 服务
func (m *Manager) createPubSubService(realm *realmImpl) (pkgif.PubSub, error) {
	// 使用 NewForRealm 创建绑定到特定 Realm 的服务
	svc, err := pubsub.NewForRealm(m.host, realm)
	if err != nil {
		return nil, err
	}

	// Phase 8 修复：如果有健康监控器，设置到 PubSub 用于错误上报
	if m.healthMonitor != nil {
		svc.SetHealthMonitor(m.healthMonitor)
	}

	return svc, nil
}

// createStreamsService 创建绑定到 Realm 的 Streams 服务
func (m *Manager) createStreamsService(realm *realmImpl) (pkgif.Streams, error) {
	// 使用 NewForRealm 创建绑定到特定 Realm 的服务
	return streams.NewForRealm(m.host, realm)
}

// createLivenessService 创建绑定到 Realm 的 Liveness 服务
func (m *Manager) createLivenessService(realm *realmImpl) (pkgif.Liveness, error) {
	// 使用 NewForRealm 创建绑定到特定 Realm 的服务
	return liveness.NewForRealm(m.host, realm)
}

// ============================================================================
//                              Protocol 服务生命周期
// ============================================================================

// startProtocolServices 启动 Realm 的所有 Protocol 服务
func startProtocolServices(ctx context.Context, realm *realmImpl) error {
	// 启动 Messaging
	if realm.messaging != nil {
		if starter, ok := realm.messaging.(interface{ Start(context.Context) error }); ok {
			if err := starter.Start(ctx); err != nil {
				return fmt.Errorf("failed to start messaging: %w", err)
			}
		}
	}

	// 启动 PubSub
	if realm.pubsub != nil {
		if starter, ok := realm.pubsub.(interface{ Start(context.Context) error }); ok {
			if err := starter.Start(ctx); err != nil {
				return fmt.Errorf("failed to start pubsub: %w", err)
			}
		}
	}

	// 启动 Streams
	if realm.streams != nil {
		if starter, ok := realm.streams.(interface{ Start(context.Context) error }); ok {
			if err := starter.Start(ctx); err != nil {
				return fmt.Errorf("failed to start streams: %w", err)
			}
		}
	}

	// 启动 Liveness
	if realm.liveness != nil {
		if starter, ok := realm.liveness.(interface{ Start(context.Context) error }); ok {
			if err := starter.Start(ctx); err != nil {
				return fmt.Errorf("failed to start liveness: %w", err)
			}
		}
	}

	return nil
}

// stopProtocolServices 停止 Realm 的所有 Protocol 服务
func stopProtocolServices(ctx context.Context, realm *realmImpl) error {
	var lastErr error

	// 停止 Liveness（先停止监控）
	if realm.liveness != nil {
		if stopper, ok := realm.liveness.(interface{ Stop(context.Context) error }); ok {
			if err := stopper.Stop(ctx); err != nil {
				lastErr = err
			}
		}
	}

	// 停止 Streams
	if realm.streams != nil {
		if stopper, ok := realm.streams.(interface{ Stop(context.Context) error }); ok {
			if err := stopper.Stop(ctx); err != nil {
				lastErr = err
			}
		}
	}

	// 停止 PubSub
	if realm.pubsub != nil {
		if stopper, ok := realm.pubsub.(interface{ Stop() error }); ok {
			if err := stopper.Stop(); err != nil {
				lastErr = err
			}
		}
	}

	// 停止 Messaging
	if realm.messaging != nil {
		if stopper, ok := realm.messaging.(interface{ Stop(context.Context) error }); ok {
			if err := stopper.Stop(ctx); err != nil {
				lastErr = err
			}
		}
	}

	return lastErr
}
