package swarm

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
)

// connectednessEventEmitter 发送 PeerConnectednessChanged 事件。
// 我们确保对于任何已连接的对等节点，在断开连接后至少发送一次 NotConnected 事件。
// 这是因为对等节点可能在收到对等节点连接状态变更事件通知之前就观察到了连接。
type connectednessEventEmitter struct {
	mx sync.RWMutex
	// newConns 是保存最近连接的对等节点 ID 的通道
	newConns      chan peer.ID
	removeConnsMx sync.Mutex
	// removeConns 是最近关闭连接的对等节点 ID 的切片
	removeConns []peer.ID
	// lastEvent 记录每个对等节点最后一次发送的连接状态事件
	lastEvent map[peer.ID]network.Connectedness
	// connectedness 是获取对等节点当前连接状态的函数
	connectedness func(peer.ID) network.Connectedness
	// emitter 是 PeerConnectednessChanged 事件发射器
	emitter         event.Emitter
	wg              sync.WaitGroup
	removeConnNotif chan struct{}
	ctx             context.Context
	cancel          context.CancelFunc
}

// newConnectednessEventEmitter 创建一个新的连接状态事件发射器
// 参数:
//   - connectedness: func(peer.ID) network.Connectedness 获取对等节点连接状态的函数
//   - emitter: event.Emitter 事件发射器
//
// 返回值:
//   - *connectednessEventEmitter 连接状态事件发射器实例
func newConnectednessEventEmitter(connectedness func(peer.ID) network.Connectedness, emitter event.Emitter) *connectednessEventEmitter {
	ctx, cancel := context.WithCancel(context.Background())
	c := &connectednessEventEmitter{
		newConns:        make(chan peer.ID, 32),
		lastEvent:       make(map[peer.ID]network.Connectedness),
		removeConnNotif: make(chan struct{}, 1),
		connectedness:   connectedness,
		emitter:         emitter,
		ctx:             ctx,
		cancel:          cancel,
	}
	c.wg.Add(1)
	go c.runEmitter()
	return c
}

// AddConn 添加一个新的连接
// 参数:
//   - p: peer.ID 对等节点 ID
func (c *connectednessEventEmitter) AddConn(p peer.ID) {
	c.mx.RLock()
	defer c.mx.RUnlock()
	if c.ctx.Err() != nil {
		return
	}

	c.newConns <- p
}

// RemoveConn 移除一个连接
// 参数:
//   - p: peer.ID 对等节点 ID
func (c *connectednessEventEmitter) RemoveConn(p peer.ID) {
	c.mx.RLock()
	defer c.mx.RUnlock()
	if c.ctx.Err() != nil {
		return
	}

	c.removeConnsMx.Lock()
	// 此队列大致受限于我们添加的连接总数。
	// 如果连接状态事件的消费者处理较慢，我们会对 AddConn 操作施加背压。
	//
	// 我们故意不在这里阻塞/施加背压以避免死锁，因为事件的消费者可能需要移除连接。
	c.removeConns = append(c.removeConns, p)

	c.removeConnsMx.Unlock()

	select {
	case c.removeConnNotif <- struct{}{}:
	default:
	}
}

// Close 关闭事件发射器
func (c *connectednessEventEmitter) Close() {
	c.cancel()
	c.wg.Wait()
}

// runEmitter 运行事件发射循环
func (c *connectednessEventEmitter) runEmitter() {
	defer c.wg.Done()
	for {
		select {
		case p := <-c.newConns:
			c.notifyPeer(p, true)
		case <-c.removeConnNotif:
			c.sendConnRemovedNotifications()
		case <-c.ctx.Done():
			c.mx.Lock() // 等待所有待处理的 AddConn 和 RemoveConn 操作完成
			defer c.mx.Unlock()
			for {
				select {
				case p := <-c.newConns:
					c.notifyPeer(p, true)
				case <-c.removeConnNotif:
					c.sendConnRemovedNotifications()
				default:
					return
				}
			}
		}
	}
}

// notifyPeer 使用发射器发送对等节点连接状态事件
// 参数:
//   - p: peer.ID 对等节点 ID
//   - forceNotConnectedEvent: bool 即使没有为此对等节点发送 Connected 事件，也强制发送 NotConnected 事件
//
// 注意:
//   - 如果在我们发送 Connected 事件之前对等节点断开连接，我们仍会发送 Disconnected 事件，因为在这种情况下可以观察到与对等节点的连接
func (c *connectednessEventEmitter) notifyPeer(p peer.ID, forceNotConnectedEvent bool) {
	oldState := c.lastEvent[p]
	c.lastEvent[p] = c.connectedness(p)
	if c.lastEvent[p] == network.NotConnected {
		delete(c.lastEvent, p)
	}
	if (forceNotConnectedEvent && c.lastEvent[p] == network.NotConnected) || c.lastEvent[p] != oldState {
		c.emitter.Emit(event.EvtPeerConnectednessChanged{
			Peer:          p,
			Connectedness: c.lastEvent[p],
		})
	}
}

// sendConnRemovedNotifications 发送连接移除通知
func (c *connectednessEventEmitter) sendConnRemovedNotifications() {
	c.removeConnsMx.Lock()
	removeConns := c.removeConns
	c.removeConns = nil
	c.removeConnsMx.Unlock()
	for _, p := range removeConns {
		c.notifyPeer(p, false)
	}
}
