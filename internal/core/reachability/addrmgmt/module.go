// Package addrmgmt 提供地址管理协议的实现
package addrmgmt

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"go.uber.org/fx"
)

// ============================================================================
//                              Fx 模块（P1 修复完成）
// ============================================================================

// Module 返回地址管理模块
func Module() fx.Option {
	return fx.Module("addrmgmt",
		fx.Provide(provideService),
		fx.Invoke(registerLifecycle),
	)
}

// ServiceParams 服务参数
type ServiceParams struct {
	fx.In

	Host      interfaces.Host     `optional:"true"`
	Peerstore interfaces.Peerstore `optional:"true"`
	EventBus  interfaces.EventBus  `optional:"true"` // 用于订阅地址变更事件
	Config    *ServiceConfig       `optional:"true"`
}

// ServiceResult 服务结果
type ServiceResult struct {
	fx.Out

	Service *Service
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	// Enabled 是否启用
	Enabled bool

	// SchedulerConfig 调度器配置
	SchedulerConfig SchedulerConfig
}

// DefaultServiceConfig 默认服务配置
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		Enabled:         true,
		SchedulerConfig: DefaultSchedulerConfig(),
	}
}

// Service 地址管理服务
//
// 封装 Handler 和 Scheduler，提供完整的地址管理功能。
type Service struct {
	handler   *Handler
	scheduler *Scheduler
	host      interfaces.Host
	eventBus  interfaces.EventBus
	config    *ServiceConfig

	// 地址变更订阅（修复 B26 数据竞争：使用 mu 保护）
	mu      sync.Mutex
	addrSub interfaces.Subscription
}

// provideService 提供地址管理服务
func provideService(params ServiceParams) (ServiceResult, error) {
	if params.Host == nil {
		logger.Debug("Host 未提供，跳过地址管理服务")
		return ServiceResult{}, nil
	}

	config := params.Config
	if config == nil {
		config = DefaultServiceConfig()
	}

	if !config.Enabled {
		logger.Debug("地址管理服务已禁用")
		return ServiceResult{}, nil
	}

	localID := params.Host.ID()

	// 创建 Handler
	handler := NewHandler(localID)

	// 创建 Scheduler
	scheduler := NewScheduler(config.SchedulerConfig, localID, handler)

	// 设置邻居函数
	scheduler.SetNeighborFuncs(
		func() []string {
			// 从 Peerstore 获取已知节点
			if params.Peerstore != nil {
				peers := params.Peerstore.PeersWithAddrs()
				result := make([]string, len(peers))
				for i, p := range peers {
					result[i] = string(p)
				}
				return result
			}
			return nil
		},
		func(ctx context.Context, peerID string, protocolID string) (interfaces.Stream, error) {
			return params.Host.NewStream(ctx, peerID, protocolID)
		},
	)

	service := &Service{
		handler:   handler,
		scheduler: scheduler,
		host:      params.Host,
		eventBus:  params.EventBus,
		config:    config,
	}

	logger.Info("地址管理服务已创建")
	return ServiceResult{Service: service}, nil
}

// lifecycleParams 生命周期依赖参数
type lifecycleParams struct {
	fx.In

	Lc      fx.Lifecycle
	Service *Service `optional:"true"` // Service 可能为 nil（禁用时）
}

// registerLifecycle 注册生命周期
func registerLifecycle(params lifecycleParams) {
	if params.Service == nil {
		return
	}

	params.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return params.Service.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return params.Service.Stop()
		},
	})
}

// ============================================================================
//                              Service 方法
// ============================================================================

// Start 启动服务
func (s *Service) Start(ctx context.Context) error {
	// 注册协议处理器
	s.host.SetStreamHandler(ProtocolID, s.handler.HandleStream)

	// 启动调度器
	if err := s.scheduler.Start(ctx); err != nil {
		return err
	}

	// 初始化本地地址（Addrs() 已返回 []string）
	s.scheduler.UpdateLocalAddrs(s.host.Addrs())

	// 订阅地址变更事件
	if s.eventBus != nil {
		sub, err := s.eventBus.Subscribe(&types.EvtLocalAddrsUpdated{})
		if err != nil {
			logger.Warn("订阅地址变更事件失败", "err", err)
		} else {
			// 使用锁保护写入（修复 B26 数据竞争）
			s.mu.Lock()
			s.addrSub = sub
			s.mu.Unlock()
			go s.handleAddrUpdates(ctx)
			logger.Debug("已订阅地址变更事件")
		}
	}

	logger.Info("地址管理服务已启动")
	return nil
}

// handleAddrUpdates 处理地址变更事件
func (s *Service) handleAddrUpdates(ctx context.Context) {
	// 获取订阅的本地引用（修复 B26 数据竞争）
	s.mu.Lock()
	sub := s.addrSub
	s.mu.Unlock()

	if sub == nil {
		return
	}

	ch := sub.Out()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if addrEvt, ok := evt.(*types.EvtLocalAddrsUpdated); ok {
				logger.Debug("收到地址变更事件", "addrs", len(addrEvt.Current))
				s.scheduler.UpdateLocalAddrs(addrEvt.Current)
			}
		}
	}
}

// Stop 停止服务
func (s *Service) Stop() error {
	// 取消地址变更订阅（修复 B26 数据竞争）
	s.mu.Lock()
	sub := s.addrSub
	s.addrSub = nil
	s.mu.Unlock()

	if sub != nil {
		sub.Close()
	}

	// 移除协议处理器
	s.host.RemoveStreamHandler(ProtocolID)

	// 停止调度器
	if err := s.scheduler.Stop(); err != nil {
		return err
	}

	logger.Info("地址管理服务已停止")
	return nil
}

// UpdateAddrs 更新本地地址
func (s *Service) UpdateAddrs(addrs []string) {
	s.scheduler.UpdateLocalAddrs(addrs)
}

// GetLocalRecord 获取本地地址记录
func (s *Service) GetLocalRecord() *AddressRecord {
	return s.scheduler.GetLocalRecord()
}

// QueryPeerAddrs 查询节点地址
func (s *Service) QueryPeerAddrs(ctx context.Context, peerID string) ([]string, error) {
	return s.scheduler.QueryPeerAddrs(ctx, peerID)
}

// GetRecord 获取缓存的地址记录
func (s *Service) GetRecord(nodeID string) *AddressRecord {
	return s.handler.GetRecord(nodeID)
}

// Handler 获取协议处理器
func (s *Service) Handler() *Handler {
	return s.handler
}

// Scheduler 获取调度器
func (s *Service) Scheduler() *Scheduler {
	return s.scheduler
}
