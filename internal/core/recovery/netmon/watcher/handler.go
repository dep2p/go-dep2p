package watcher

import (
	"context"

	"go.uber.org/fx"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// logger 在 doc.go 中定义

// Handler 网络变化处理器
type Handler struct {
	monitor      pkgif.NetworkMonitor
	natService   NATService
	relayManager RelayManager
	discovery    DiscoveryService
	transport    TransportService
	dnsService   DNSService
	endpoints    EndpointService
	realmNotify  RealmNotifier

	// 生命周期控制
	ctx    context.Context
	cancel context.CancelFunc
}

// ============================================================================
//                              内部接口定义
// ============================================================================
// 这些接口与 pkg/interfaces 中的公共接口对齐，
// 用于类型别名以保持向后兼容性。

// NATService NAT 服务接口（等同于 pkgif.NATRefresher）
type NATService = pkgif.NATRefresher

// RelayManager 中继管理器接口（等同于 pkgif.RelayConnectionManager）
type RelayManager = pkgif.RelayConnectionManager

// DiscoveryService 发现服务接口（等同于 pkgif.DiscoveryAddressUpdater）
type DiscoveryService = pkgif.DiscoveryAddressUpdater

// TransportService 传输服务接口（等同于 pkgif.TransportRebinder）
type TransportService = pkgif.TransportRebinder

// DNSService DNS 服务接口（等同于 pkgif.DNSResetter）
//
// 参考 iroh-main: 网络变化时 DNS 解析器的缓存可能失效，
// 新网络可能使用不同的 DNS 服务器。
type DNSService = pkgif.DNSResetter

// EndpointService 端点服务接口（等同于 pkgif.EndpointStateResetter）
//
// 参考 iroh-main: 网络变化时已建立的端点路径可能失效，
// 需要重置端点状态以触发重新发现最优路径。
type EndpointService = pkgif.EndpointStateResetter

// RealmNotifier Realm 通知接口（等同于 pkgif.RealmNetworkNotifier）
//
// 通知所有活跃的 Realm 网络已变化，
// 触发 Capability 重新广播和成员地址刷新。
type RealmNotifier = pkgif.RealmNetworkNotifier

// HandlerParams 处理器依赖参数
type HandlerParams struct {
	fx.In

	Monitor      pkgif.NetworkMonitor `optional:"true"`
	NATService   NATService           `optional:"true"`
	RelayManager RelayManager         `optional:"true"`
	Discovery    DiscoveryService     `optional:"true"`
	Transport    TransportService     `optional:"true"`
	DNSService   DNSService           `optional:"true"`
	Endpoints    EndpointService      `optional:"true"`
	RealmNotify  RealmNotifier        `optional:"true"`
}

// NewHandler 创建网络变化处理器
func NewHandler(params HandlerParams) *Handler {
	return &Handler{
		monitor:      params.Monitor,
		natService:   params.NATService,
		relayManager: params.RelayManager,
		discovery:    params.Discovery,
		transport:    params.Transport,
		dnsService:   params.DNSService,
		endpoints:    params.Endpoints,
		realmNotify:  params.RealmNotify,
	}
}

// Start 启动处理器
func (h *Handler) Start(_ context.Context) error {
	if h.monitor == nil {
		logger.Warn("网络监控器未设置，跳过网络变化处理")
		return nil
	}

	// 使用 context.Background() 而不是传入的 ctx
	// 因为 Fx OnStart 的 ctx 在返回后会被取消，导致后台循环提前退出
	h.ctx, h.cancel = context.WithCancel(context.Background())

	// 订阅网络变化事件
	events := h.monitor.Subscribe()

	// 启动事件处理循环
	go h.eventLoop(h.ctx, events)

	logger.Info("网络变化处理器已启动")
	return nil
}

// Stop 停止处理器
func (h *Handler) Stop() error {
	if h.cancel != nil {
		h.cancel()
	}
	logger.Info("网络变化处理器已停止")
	return nil
}

// eventLoop 事件处理循环
func (h *Handler) eventLoop(ctx context.Context, events <-chan pkgif.NetworkChangeEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			h.HandleNetworkChange(ctx, event)
		}
	}
}

// HandleNetworkChange 处理网络变化事件
func (h *Handler) HandleNetworkChange(ctx context.Context, event pkgif.NetworkChangeEvent) {
	logger.Info("处理网络变化",
		"type", event.Type,
		"oldAddrs", len(event.OldAddrs),
		"newAddrs", len(event.NewAddrs))

	if event.Type == pkgif.NetworkChangeMajor {
		h.handleMajorChange(ctx, event)
	} else {
		h.handleMinorChange(ctx, event)
	}
}

// handleMajorChange 处理主要网络变化
//
// 参考 iroh-main 的 handle_network_change 实现：
// 完整的 Major Change 处理流程包括 7 个步骤。
func (h *Handler) handleMajorChange(ctx context.Context, event pkgif.NetworkChangeEvent) {
	logger.Info("处理主要网络变化（接口切换）")

	// 1. Socket Rebind - 重新绑定传输层套接字
	if h.transport != nil {
		if err := h.transport.Rebind(ctx); err != nil {
			logger.Warn("Socket rebind 失败", "err", err)
		} else {
			logger.Debug("Socket rebind 成功")
		}
	}

	// 2. DNS Reset - 重置 DNS 解析器
	// 新网络可能使用不同的 DNS 服务器，需要清除旧缓存
	if h.dnsService != nil {
		if err := h.dnsService.Reset(ctx); err != nil {
			logger.Warn("DNS reset 失败", "err", err)
		} else {
			logger.Debug("DNS reset 成功")
		}
	}

	// 3. Re-STUN - 重新执行 STUN 探测获取新的外部地址
	if h.natService != nil {
		if err := h.natService.ForceSTUN(ctx); err != nil {
			logger.Warn("Re-STUN 失败", "err", err)
		} else {
			logger.Debug("Re-STUN 成功")
		}
	}

	// 4. Close stale relay connections - 关闭失效的中继连接
	if h.relayManager != nil {
		if err := h.relayManager.CloseStaleConnections(ctx); err != nil {
			logger.Warn("关闭失效中继连接失败", "err", err)
		} else {
			logger.Debug("关闭失效中继连接成功")
		}
	}

	// 5. Reset Endpoints - 重置端点状态，清除旧的最佳路径
	if h.endpoints != nil {
		if err := h.endpoints.ResetStates(ctx); err != nil {
			logger.Warn("重置端点状态失败", "err", err)
		} else {
			logger.Debug("重置端点状态成功")
		}
	}

	// 6. Update DHT - 更新 DHT 中的地址
	if h.discovery != nil {
		if err := h.discovery.UpdateAddrs(ctx); err != nil {
			logger.Warn("更新 DHT 地址失败", "err", err)
		} else {
			logger.Debug("更新 DHT 地址成功")
		}
	}

	// 7. Notify Realm - 通知所有活跃的 Realm
	// 触发 Capability 重新广播和成员地址刷新
	if h.realmNotify != nil {
		if err := h.realmNotify.NotifyNetworkChange(ctx, event); err != nil {
			logger.Warn("通知 Realm 失败", "err", err)
		} else {
			logger.Debug("通知 Realm 成功")
		}
	}

	logger.Info("主要网络变化处理完成")
}

// handleMinorChange 处理次要网络变化
func (h *Handler) handleMinorChange(ctx context.Context, event pkgif.NetworkChangeEvent) {
	logger.Info("处理次要网络变化（IP 地址变化）")

	// 1. Re-STUN
	if h.natService != nil {
		if err := h.natService.ForceSTUN(ctx); err != nil {
			logger.Warn("Re-STUN 失败", "err", err)
		} else {
			logger.Debug("Re-STUN 成功")
		}
	}

	// 2. Update DHT (if address changed)
	if len(event.NewAddrs) > 0 && !equalAddrs(event.OldAddrs, event.NewAddrs) {
		if h.discovery != nil {
			if err := h.discovery.UpdateAddrs(ctx); err != nil {
				logger.Warn("更新 DHT 地址失败", "err", err)
			} else {
				logger.Debug("更新 DHT 地址成功")
			}
		}
	}

	logger.Info("次要网络变化处理完成")
}
