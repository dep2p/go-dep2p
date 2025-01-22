package pstoremanager

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"

	logging "github.com/dep2p/log"
)

// log 日志记录器
var log = logging.Logger("host-pstoremanager")

// Option 定义配置选项函数类型
type Option func(*PeerstoreManager) error

// WithGracePeriod 设置宽限期
// 参数:
//   - p: 宽限期时长
//
// 返回:
//   - Option: 配置选项函数
func WithGracePeriod(p time.Duration) Option {
	return func(m *PeerstoreManager) error {
		m.gracePeriod = p // 设置宽限期
		return nil
	}
}

// WithCleanupInterval 设置清理间隔
// 参数:
//   - t: 清理间隔时长
//
// 返回:
//   - Option: 配置选项函数
func WithCleanupInterval(t time.Duration) Option {
	return func(m *PeerstoreManager) error {
		m.cleanupInterval = t // 设置清理间隔
		return nil
	}
}

// PeerstoreManager 对等节点存储管理器结构体
type PeerstoreManager struct {
	pstore   peerstore.Peerstore // 对等节点存储
	eventBus event.Bus           // 事件总线
	network  network.Network     // 网络接口

	cancel   context.CancelFunc // 取消函数
	refCount sync.WaitGroup     // 引用计数器

	gracePeriod     time.Duration // 宽限期
	cleanupInterval time.Duration // 清理间隔
}

// NewPeerstoreManager 创建新的对等节点存储管理器
// 参数:
//   - pstore: 对等节点存储
//   - eventBus: 事件总线
//   - network: 网络接口
//   - opts: 配置选项
//
// 返回:
//   - *PeerstoreManager: 管理器实例
//   - error: 错误信息
func NewPeerstoreManager(pstore peerstore.Peerstore, eventBus event.Bus, network network.Network, opts ...Option) (*PeerstoreManager, error) {
	m := &PeerstoreManager{
		pstore:      pstore,      // 设置对等节点存储
		gracePeriod: time.Minute, // 默认宽限期为1分钟
		eventBus:    eventBus,    // 设置事件总线
		network:     network,     // 设置网络接口
	}
	for _, opt := range opts { // 应用配置选项
		if err := opt(m); err != nil {
			log.Debugf("应用配置选项失败: %v", err)
			return nil, err
		}
	}
	if m.cleanupInterval == 0 { // 如果未设置清理间隔
		m.cleanupInterval = m.gracePeriod / 2 // 设置为宽限期的一半
	}
	return m, nil
}

// Start 启动对等节点存储管理器
func (m *PeerstoreManager) Start() {
	ctx, cancel := context.WithCancel(context.Background())                                                // 创建可取消的上下文
	m.cancel = cancel                                                                                      // 保存取消函数
	sub, err := m.eventBus.Subscribe(&event.EvtPeerConnectednessChanged{}, eventbus.Name("pstoremanager")) // 订阅连接状态变更事件
	if err != nil {
		log.Debugf("订阅失败。对等节点存储管理器未激活。错误: %s", err)
		return
	}
	m.refCount.Add(1)         // 增加引用计数
	go m.background(ctx, sub) // 启动后台任务
}

// background 后台运行清理任务
// 参数:
//   - ctx: 上下文
//   - sub: 事件订阅
func (m *PeerstoreManager) background(ctx context.Context, sub event.Subscription) {
	defer m.refCount.Done()                     // 减少引用计数
	defer sub.Close()                           // 关闭订阅
	disconnected := make(map[peer.ID]time.Time) // 断开连接的节点映射表

	ticker := time.NewTicker(m.cleanupInterval) // 创建定时器
	defer ticker.Stop()                         // 停止定时器

	defer func() { // 退出时清理
		for p := range disconnected { // 遍历断开连接的节点
			m.pstore.RemovePeer(p) // 移除节点
		}
	}()

	for {
		select {
		case e, ok := <-sub.Out(): // 接收事件
			if !ok {
				return
			}
			ev := e.(event.EvtPeerConnectednessChanged) // 类型断言
			p := ev.Peer                                // 获取节点ID
			switch ev.Connectedness {                   // 根据连接状态处理
			case network.Connected, network.Limited: // 已连接或受限状态
				delete(disconnected, p) // 从断开映射中删除
			default: // 其他状态
				if _, ok := disconnected[p]; !ok { // 如果不在断开映射中
					disconnected[p] = time.Now() // 记录断开时间
				}
			}
		case <-ticker.C: // 定时器触发
			now := time.Now()                             // 获取当前时间
			for p, disconnectTime := range disconnected { // 遍历断开的节点
				if disconnectTime.Add(m.gracePeriod).Before(now) { // 如果超过宽限期
					switch m.network.Connectedness(p) { // 检查当前连接状态
					case network.Connected, network.Limited: // 已重新连接
					default: // 仍然断开
						m.pstore.RemovePeer(p) // 移除节点
					}
					delete(disconnected, p) // 从映射中删除
				}
			}
		case <-ctx.Done(): // 上下文取消
			return
		}
	}
}

// Close 关闭对等节点存储管理器
// 返回:
//   - error: 错误信息
func (m *PeerstoreManager) Close() error {
	if m.cancel != nil { // 如果存在取消函数
		m.cancel() // 执行取消
	}
	m.refCount.Wait() // 等待所有任务完成
	return nil
}
