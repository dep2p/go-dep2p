package relaysvc

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"
	relayv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/relay"
	logging "github.com/dep2p/log"
)

var log = logging.Logger("host-relaysvc")

// RelayManager 中继管理器结构体
type RelayManager struct {
	host host.Host // 主机实例

	mutex sync.Mutex       // 互斥锁
	relay *relayv2.Relay   // 中继实例
	opts  []relayv2.Option // 中继选项

	refCount  sync.WaitGroup     // 引用计数器
	ctxCancel context.CancelFunc // 上下文取消函数
}

// NewRelayManager 创建新的中继管理器
// 参数:
//   - host: 主机实例
//   - opts: 中继选项
//
// 返回:
//   - *RelayManager: 中继管理器实例
func NewRelayManager(host host.Host, opts ...relayv2.Option) *RelayManager {
	ctx, cancel := context.WithCancel(context.Background()) // 创建可取消的上下文
	m := &RelayManager{
		host:      host,   // 设置主机
		opts:      opts,   // 设置选项
		ctxCancel: cancel, // 设置取消函数
	}
	m.refCount.Add(1)    // 增加引用计数
	go m.background(ctx) // 启动后台任务
	return m             // 返回管理器实例
}

// background 后台运行中继管理任务
// 参数:
//   - ctx: 上下文
func (m *RelayManager) background(ctx context.Context) {
	defer m.refCount.Done() // 减少引用计数
	defer func() {          // 退出时清理
		m.mutex.Lock()      // 加锁
		if m.relay != nil { // 如果中继存在
			m.relay.Close() // 关闭中继
		}
		m.mutex.Unlock() // 解锁
	}()

	subReachability, _ := m.host.EventBus().Subscribe(new(event.EvtLocalReachabilityChanged), eventbus.Name("relaysvc")) // 订阅可达性变更事件
	defer subReachability.Close()                                                                                        // 关闭订阅

	for {
		select {
		case <-ctx.Done(): // 上下文取消
			return
		case ev, ok := <-subReachability.Out(): // 接收事件
			if !ok { // 通道关闭
				return
			}
			if err := m.reachabilityChanged(ev.(event.EvtLocalReachabilityChanged).Reachability); err != nil { // 处理可达性变更
				log.Debugf("处理可达性变更失败: %v", err)
				return
			}
		}
	}
}

// reachabilityChanged 处理可达性变更
// 参数:
//   - r: 可达性状态
//
// 返回:
//   - error: 错误信息
func (m *RelayManager) reachabilityChanged(r network.Reachability) error {
	switch r {
	case network.ReachabilityPublic: // 公网可达
		m.mutex.Lock()         // 加锁
		defer m.mutex.Unlock() // 解锁
		// 如果两个连续的可达性变更事件报告相同状态可能发生
		// 这不应该发生,但为安全起见需要双重检查
		if m.relay != nil { // 如果中继已存在
			return nil
		}
		relay, err := relayv2.New(m.host, m.opts...) // 创建新的中继
		if err != nil {                              // 创建失败
			log.Debugf("创建中继失败: %v", err)
			return err
		}
		m.relay = relay // 设置中继
	default: // 其他可达性状态
		m.mutex.Lock()         // 加锁
		defer m.mutex.Unlock() // 解锁
		if m.relay != nil {    // 如果中继存在
			err := m.relay.Close() // 关闭中继
			m.relay = nil          // 清空中继
			log.Debugf("关闭中继失败: %v", err)
			return err
		}
	}
	return nil
}

// Close 关闭中继管理器
// 返回:
//   - error: 错误信息
func (m *RelayManager) Close() error {
	m.ctxCancel()     // 取消上下文
	m.refCount.Wait() // 等待所有任务完成
	return nil
}
