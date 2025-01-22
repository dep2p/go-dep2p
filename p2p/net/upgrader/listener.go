package upgrader

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/transport"

	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	logging "github.com/dep2p/log"
	tec "github.com/jbenet/go-temp-err-catcher"
)

// log 是用于记录日志的 logger 实例
var log = logging.Logger("net-upgrader")

// listener 实现了 transport.Listener 接口
// 用于处理传入的网络连接并进行升级
type listener struct {
	// Listener 是底层的网络监听器
	manet.Listener

	// transport 是用于处理连接的传输层实现
	transport transport.Transport
	// upgrader 用于升级连接
	upgrader *upgrader
	// rcmgr 用于资源管理
	rcmgr network.ResourceManager

	// incoming 用于传递已升级的连接
	incoming chan transport.CapableConn
	// err 存储监听器的错误状态
	err error

	// threshold 用于实现背压机制
	threshold *threshold

	// ctx 和 cancel 用于控制监听器的生命周期
	// 注意: 仅取消 context 不足以关闭监听器,需要调用 close
	ctx    context.Context
	cancel func()
}

// Close 关闭监听器
// 返回值:
//   - error 关闭过程中的错误
func (l *listener) Close() error {
	// 首先关闭底层监听器以获取相关错误
	err := l.Listener.Close()

	// 取消 context
	l.cancel()
	// 清空并等待所有连接关闭
	for c := range l.incoming {
		c.Close()
	}
	return err
}

// handleIncoming 处理传入的连接
//
// 该函数有以下几个重要特性:
//
//  1. 记录并丢弃临时/瞬态错误(具有返回 true 的 Temporary() 函数的错误)
//  2. 当已完全协商但未被接受的连接数达到 AcceptQueueLength 时,停止接受新连接
//     这提供了基本的背压机制,同时仍允许并行协商连接
func (l *listener) handleIncoming() {
	var wg sync.WaitGroup
	defer func() {
		// 确保监听器已关闭
		l.Listener.Close()
		if l.err == nil {
			l.err = fmt.Errorf("监听器已关闭")
		}

		wg.Wait()
		close(l.incoming)
	}()

	var catcher tec.TempErrCatcher
	for l.ctx.Err() == nil {
		maconn, err := l.Listener.Accept()
		if err != nil {
			// 注意: 函数可能暂停接受循环
			if catcher.IsTemporary(err) {
				// log.Infof("临时接受错误: %s", err)
				continue
			}
			l.err = err
			return
		}
		catcher.Reset()

		// 检查是否已有连接作用域。详见 tcpreuse/listener.go 中的说明
		var connScope network.ConnManagementScope
		if sc, ok := maconn.(interface {
			Scope() network.ConnManagementScope
		}); ok {
			connScope = sc.Scope()
		}
		if connScope == nil {
			// 如果适用,对连接进行过滤
			if l.upgrader.connGater != nil && !l.upgrader.connGater.InterceptAccept(maconn) {
				log.Debugf("过滤器阻止了从 %s 到本地地址 %s 的传入连接",
					maconn.RemoteMultiaddr(), maconn.LocalMultiaddr())
				if err := maconn.Close(); err != nil {
					log.Warnf("关闭被过滤器拒绝的传入连接失败: %s", err)
				}
				continue
			}

			var err error
			connScope, err = l.rcmgr.OpenConnection(network.DirInbound, true, maconn.RemoteMultiaddr())
			if err != nil {
				log.Debugw("资源管理器阻止接受新连接", "错误", err)
				if err := maconn.Close(); err != nil {
					log.Warnf("打开传入连接失败。被资源管理器拒绝: %s", err)
				}
				continue
			}
		}

		// 当 context 被取消时,下面的 goroutine 会调用 Release,因此这里无需等待
		l.threshold.Wait()

		log.Debugf("监听器 %s 获得连接: %s <---> %s",
			l,
			maconn.LocalMultiaddr(),
			maconn.RemoteMultiaddr())

		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(l.ctx, l.upgrader.acceptTimeout)
			defer cancel()

			conn, err := l.upgrader.Upgrade(ctx, l.transport, maconn, network.DirInbound, "", connScope)
			if err != nil {
				// 无需向上传递此错误。我们只是未能完全协商连接
				log.Debugf("接受升级错误: %s (%s <--> %s)",
					err,
					maconn.LocalMultiaddr(),
					maconn.RemoteMultiaddr())
				connScope.Done()
				return
			}

			log.Debugf("监听器 %s 接受连接: %s", l, conn)

			// 记录连接已设置完成并等待被接受
			// 此调用永远不会阻塞,即使我们超过阈值
			// 它只是确保当我们超过阈值时 Wait 调用会阻塞
			l.threshold.Acquire()
			defer l.threshold.Release()

			select {
			case l.incoming <- conn:
			case <-ctx.Done():
				if l.ctx.Err() == nil {
					// 监听器未关闭但接受超时已过期
					log.Warn("由于接受过慢,监听器丢弃了连接")
				}
				// 使用超时等待 context。这样,如果我们因某种原因停止接受连接,我们最终会关闭所有打开的连接,而不是一直保持它们
				conn.Close()
			}
		}()
	}
}

// Accept 接受一个连接
// 返回值:
//   - transport.CapableConn 已升级的传输层连接
//   - error 接受过程中的错误
func (l *listener) Accept() (transport.CapableConn, error) {
	for c := range l.incoming {
		// 可能已在队列中等待了一段时间
		if !c.IsClosed() {
			return c, nil
		}
	}
	if strings.Contains(l.err.Error(), "使用已关闭的网络连接") {
		log.Debugf("监听器已关闭")
		return nil, transport.ErrListenerClosed
	}
	return nil, l.err
}

// String 返回监听器的字符串表示
// 返回值:
//   - string 监听器的描述字符串
func (l *listener) String() string {
	if s, ok := l.transport.(fmt.Stringer); ok {
		return fmt.Sprintf("<stream.Listener[%s] %s>", s, l.Multiaddr())
	}
	return fmt.Sprintf("<stream.Listener %s>", l.Multiaddr())
}

// 确保 listener 类型实现了 transport.Listener 接口
var _ transport.Listener = (*listener)(nil)
