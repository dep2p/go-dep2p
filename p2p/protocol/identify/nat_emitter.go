package identify

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"
)

// natEmitter 用于发送 NAT 相关事件的组件
type natEmitter struct {
	ctx             context.Context      // 上下文
	cancel          context.CancelFunc   // 取消函数
	wg              sync.WaitGroup       // 等待组
	reachabilitySub event.Subscription   // 可达性订阅
	reachability    network.Reachability // 当前可达性状态
	eventInterval   time.Duration        // 事件发送间隔

	currentUDPNATDeviceType  network.NATDeviceType // 当前 UDP NAT 设备类型
	currentTCPNATDeviceType  network.NATDeviceType // 当前 TCP NAT 设备类型
	emitNATDeviceTypeChanged event.Emitter         // NAT 设备类型变更事件发送器

	observedAddrMgr *ObservedAddrManager // 观察到的地址管理器
}

// newNATEmitter 创建一个新的 NAT 事件发送器
// 参数:
//   - h: host.Host 主机对象
//   - o: *ObservedAddrManager 观察到的地址管理器
//   - eventInterval: time.Duration 事件发送间隔
//
// 返回值:
//   - *natEmitter NAT 事件发送器
//   - error 错误信息
func newNATEmitter(h host.Host, o *ObservedAddrManager, eventInterval time.Duration) (*natEmitter, error) {
	// 创建上下文和取消函数
	ctx, cancel := context.WithCancel(context.Background())
	n := &natEmitter{
		observedAddrMgr: o,
		ctx:             ctx,
		cancel:          cancel,
		eventInterval:   eventInterval,
	}

	// 订阅可达性变更事件
	reachabilitySub, err := h.EventBus().Subscribe(new(event.EvtLocalReachabilityChanged), eventbus.Name("identify (nat emitter)"))
	if err != nil {
		log.Debugf("订阅可达性事件失败: %s", err)
		return nil, err
	}
	n.reachabilitySub = reachabilitySub

	// 创建 NAT 设备类型变更事件发送器
	emitter, err := h.EventBus().Emitter(new(event.EvtNATDeviceTypeChanged), eventbus.Stateful)
	if err != nil {
		log.Debugf("创建 NAT 设备类型事件发送器失败: %s", err)
		return nil, err
	}
	n.emitNATDeviceTypeChanged = emitter

	// 启动工作协程
	n.wg.Add(1)
	go n.worker()
	return n, nil
}

// worker 处理事件和定时任务的工作协程
func (n *natEmitter) worker() {
	defer n.wg.Done()
	subCh := n.reachabilitySub.Out()
	ticker := time.NewTicker(n.eventInterval)
	pendingUpdate := false
	enoughTimeSinceLastUpdate := true
	for {
		select {
		case evt, ok := <-subCh:
			// 处理可达性变更事件
			if !ok {
				subCh = nil
				continue
			}
			ev, ok := evt.(event.EvtLocalReachabilityChanged)
			if !ok {
				log.Debugf("无效事件: %v", evt)
				continue
			}
			n.reachability = ev.Reachability
		case <-ticker.C:
			// 处理定时更新
			enoughTimeSinceLastUpdate = true
			if pendingUpdate {
				n.maybeNotify()
				pendingUpdate = false
				enoughTimeSinceLastUpdate = false
			}
		case <-n.observedAddrMgr.addrRecordedNotif:
			// 处理地址记录通知
			pendingUpdate = true
			if enoughTimeSinceLastUpdate {
				n.maybeNotify()
				pendingUpdate = false
				enoughTimeSinceLastUpdate = false
			}
		case <-n.ctx.Done():
			return
		}
	}
}

// maybeNotify 在必要时发送 NAT 设备类型变更通知
func (n *natEmitter) maybeNotify() {
	// 仅在私有网络环境下发送通知
	if n.reachability == network.ReachabilityPrivate {
		// 获取当前 NAT 类型
		tcpNATType, udpNATType := n.observedAddrMgr.getNATType()
		// 检查并发送 TCP NAT 类型变更
		if tcpNATType != n.currentTCPNATDeviceType {
			n.currentTCPNATDeviceType = tcpNATType
			n.emitNATDeviceTypeChanged.Emit(event.EvtNATDeviceTypeChanged{
				TransportProtocol: network.NATTransportTCP,
				NatDeviceType:     n.currentTCPNATDeviceType,
			})
		}
		// 检查并发送 UDP NAT 类型变更
		if udpNATType != n.currentUDPNATDeviceType {
			n.currentUDPNATDeviceType = udpNATType
			n.emitNATDeviceTypeChanged.Emit(event.EvtNATDeviceTypeChanged{
				TransportProtocol: network.NATTransportUDP,
				NatDeviceType:     n.currentUDPNATDeviceType,
			})
		}
	}
}

// Close 关闭 NAT 事件发送器
func (n *natEmitter) Close() {
	n.cancel()                         // 取消上下文
	n.wg.Wait()                        // 等待工作协程结束
	n.reachabilitySub.Close()          // 关闭可达性订阅
	n.emitNATDeviceTypeChanged.Close() // 关闭事件发送器
}
