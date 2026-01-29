package mdns

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"go.uber.org/fx"
)

// logger 在 mdns.go 中定义

// Module 返回 Fx 模块
var Module = fx.Module("discovery/mdns",
	fx.Provide(ProvideMDNS),
	fx.Invoke(registerLifecycle),
)

// ModuleInput Fx 输入参数
type ModuleInput struct {
	fx.In
	Host       pkgif.Host     // 移除 name 标签
	EventBus   pkgif.EventBus `optional:"true"` // 可选的事件总线
	UnifiedCfg *config.Config `optional:"true"`
}

// ConfigFromUnified 从统一配置创建 mDNS 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil || !cfg.Discovery.EnableMDNS {
		return &Config{Enabled: false}
	}
	return &Config{
		ServiceTag: cfg.Discovery.MDNS.ServiceTag,
		Interval:   cfg.Discovery.MDNS.Interval.Duration(),
		Enabled:    cfg.Discovery.EnableMDNS,
	}
}

// MDNSResult MDNS 服务结果
type MDNSResult struct {
	fx.Out
	Discovery pkgif.Discovery `name:"mdns"` // 使用 name 标签避免与 DHT 冲突
	MDNS      *MDNS           // 导出具体类型以便进行地址更新订阅
}

// ProvideMDNS 提供 MDNS 服务
func ProvideMDNS(input ModuleInput) (MDNSResult, error) {
	cfg := ConfigFromUnified(input.UnifiedCfg)
	mdns, err := New(input.Host, cfg)
	if err != nil {
		return MDNSResult{}, err
	}
	return MDNSResult{
		Discovery: mdns,
		MDNS:      mdns,
	}, nil
}

// lifecycleInput 生命周期注册输入
type lifecycleInput struct {
	fx.In
	LC       fx.Lifecycle
	Host     pkgif.Host
	MDNS     *MDNS          `optional:"true"` // 由 ProvideMDNS 提供
	EventBus pkgif.EventBus `optional:"true"` // 可选的事件总线
}

// peerConnector 管理 mDNS 发现后的自动连接
type peerConnector struct {
	host     pkgif.Host
	mdns     *MDNS
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	interval time.Duration
}

// newPeerConnector 创建节点连接器
func newPeerConnector(host pkgif.Host, mdns *MDNS) *peerConnector {
	ctx, cancel := context.WithCancel(context.Background())
	return &peerConnector{
		host:     host,
		mdns:     mdns,
		ctx:      ctx,
		cancel:   cancel,
		interval: 5 * time.Second, // 每 5 秒发现一次
	}
}

// start 启动自动发现和连接
func (pc *peerConnector) start() {
	pc.wg.Add(1)
	go pc.discoverAndConnectLoop()
}

// stop 停止自动发现和连接
func (pc *peerConnector) stop() {
	pc.cancel()
	pc.wg.Wait()
}

// discoverAndConnectLoop 持续发现并连接节点
func (pc *peerConnector) discoverAndConnectLoop() {
	defer pc.wg.Done()

	// 等待 mDNS 服务启动
	pc.waitForMDNSReady()

	// 启动持续发现（长期运行的 resolver）
	pc.startContinuousDiscovery()
}

// waitForMDNSReady 等待 mDNS 服务就绪
func (pc *peerConnector) waitForMDNSReady() {
	ticker := time.NewTicker(pc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if pc.mdns.Started() && !pc.mdns.Closed() {
				// 如果处于等待状态，尝试重新启动服务器
				if pc.mdns.IsWaiting() {
					logger.Debug("处于等待状态，尝试重新启动服务器")
					if err := pc.mdns.TryStartServer(); err != nil {
						logger.Warn("重启服务器失败", "error", err)
						continue
					}
				}
				if pc.mdns.State() == StateRunning {
					logger.Info("mDNS 服务就绪，开始持续发现")
					return
				}
			}
		case <-pc.ctx.Done():
			return
		}
	}
}

// startContinuousDiscovery 启动持续发现（长期运行）
func (pc *peerConnector) startContinuousDiscovery() {
	logger.Info("启动 mDNS 持续发现")

	// 使用持续运行的 context
	peerCh, err := pc.mdns.FindPeers(pc.ctx, "_dep2p._udp")
	if err != nil {
		logger.Error("启动持续发现失败", "error", err)
		return
	}

	// 持续处理发现的节点
	for {
		select {
		case peerInfo, ok := <-peerCh:
			if !ok {
				logger.Debug("发现 channel 已关闭")
				return
			}

			// 跳过自己
			if string(peerInfo.ID) == pc.host.ID() {
				logger.Debug("跳过自己", "peerID", truncateID(string(peerInfo.ID), 8))
				continue
			}

			logger.Info("mDNS 发现节点", "peerID", truncateID(string(peerInfo.ID), 8), "addrs", len(peerInfo.Addrs))

			// 提取地址字符串
			var addrs []string
			for _, addr := range peerInfo.Addrs {
				addrs = append(addrs, addr.String())
			}

			if len(addrs) == 0 {
				logger.Debug("节点无地址，跳过", "peerID", truncateID(string(peerInfo.ID), 8))
				continue
			}

			// 尝试连接（在后台执行，不阻塞发现循环）
			go func(peerID string, peerAddrs []string) {
				connectCtx, connectCancel := context.WithTimeout(pc.ctx, 10*time.Second)
				defer connectCancel()

				err := pc.host.Connect(connectCtx, peerID, peerAddrs)
				if err != nil {
					logger.Debug("mDNS 自动连接失败",
						"peer", truncateID(peerID, 8),
						"error", err,
					)
				} else {
					logger.Info("mDNS 自动连接成功",
						"peer", truncateID(peerID, 8),
					)
				}
			}(string(peerInfo.ID), addrs)

		case <-pc.ctx.Done():
			logger.Debug("持续发现被取消")
			return
		}
	}
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	mdns := input.MDNS
	if mdns == nil {
		return // mDNS 未启用
	}

	// 创建节点连接器
	connector := newPeerConnector(input.Host, mdns)

	// 订阅地址变化事件
	var subscription pkgif.Subscription
	if input.EventBus != nil {
		sub, err := input.EventBus.Subscribe(new(EvtLocalAddrsUpdated))
		if err == nil {
			subscription = sub
			// 启动地址变化监听协程
			go handleAddressChanges(mdns, subscription)
		}
	}

	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 启动 MDNS 服务
			// 如果没有有效地址，会进入 Waiting 状态而非返回错误
			if err := mdns.Start(ctx); err != nil {
				return err
			}

			// 启动自动发现和连接
			connector.start()
			logger.Info("mDNS 自动发现已启动")

			return nil
		},
		OnStop: func(ctx context.Context) error {
			// 停止自动发现和连接
			connector.stop()

			// 关闭订阅
			if subscription != nil {
				subscription.Close()
			}
			// 停止 MDNS 服务
			return mdns.Stop(ctx)
		},
	})
}

// handleAddressChanges 处理地址变化事件
//
// 事件驱动：订阅 EvtLocalAddrsUpdated 替代轮询，减少启动延迟。
func handleAddressChanges(mdns *MDNS, sub pkgif.Subscription) {
	for evt := range sub.Out() {
		// EventBus 返回指针类型
		addrEvt, ok := evt.(*EvtLocalAddrsUpdated)
		if !ok {
			continue
		}

		logger.Debug("收到地址更新事件", "current", len(addrEvt.Current), "added", len(addrEvt.Added))

		// 根据地址变化更新 mDNS 状态
		if len(addrEvt.Current) > 0 {
			// 有地址可用，尝试启动服务器
			if mdns.IsWaiting() {
				logger.Info("地址可用，尝试启动 mDNS 服务器")
				if err := mdns.TryStartServer(); err != nil {
					logger.Warn("启动 mDNS 服务器失败", "error", err)
				}
			}
		} else {
			// 地址全部丢失，停止服务器
			if mdns.IsRunning() {
				logger.Info("地址丢失，停止 mDNS 服务器")
				mdns.stopServer()
			}
		}
	}
}
