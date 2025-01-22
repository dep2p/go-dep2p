// The udpmux package contains the logic for multiplexing multiple WebRTC (ICE) connections over a single UDP socket.
package udpmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
	logging "github.com/dep2p/log"
	"github.com/pion/ice/v2"
	"github.com/pion/stun"
)

// 用于记录日志的 logger 实例
var log = logging.Logger("webrtc-udpmux")

// ReceiveBufSize 是用于从 PacketConn 接收数据包的缓冲区大小。
// 这个值可以大于实际的路径 MTU,因为它不会用于决定写入路径上的数据包大小。
const ReceiveBufSize = 1500

// Candidate 表示一个 ICE 候选者
type Candidate struct {
	// Ufrag 是用户片段标识符
	Ufrag string
	// Addr 是候选者的 UDP 地址
	Addr *net.UDPAddr
}

// UDPMux 在单个 net.PacketConn(通常是 UDP socket)上多路复用多个 ICE 连接。
//
// 连接通过(ufrag, IP 地址族)和接收到有效 STUN/RTC 数据包的远程地址进行索引。
//
// 当在底层 net.PacketConn 上收到新数据包时,我们首先检查地址映射以查看是否有与远程地址关联的连接:
// 如果找到,我们将数据包传递给该连接。
// 否则,我们检查数据包是否为 STUN 数据包。
// 如果是,我们从 STUN 数据包中读取 ufrag 并使用它来检查是否有与(ufrag, IP 地址族)对关联的连接。
// 如果找到,我们将关联添加到地址映射中。
type UDPMux struct {
	// socket 是底层的数据包连接
	socket net.PacketConn

	// queue 是候选者队列通道
	queue chan Candidate

	// mx 是用于保护并发访问的互斥锁
	mx sync.Mutex
	// ufragMap 允许我们基于 ufrag 多路复用传入的 STUN 数据包
	ufragMap map[ufragConnKey]*muxedConnection
	// addrMap 允许我们在建立连接后正确引导传入的数据包,此时并非所有数据包都有 ufrag
	addrMap map[string]*muxedConnection
	// ufragAddrMap 允许在连接关闭时清理 addrMap 中的所有地址。在 ICE 连接性检查期间,相同的 ufrag 可能在多个地址上使用
	ufragAddrMap map[ufragConnKey][]net.Addr

	// ctx 控制多路复用器的生命周期
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// 确保 UDPMux 实现了 ice.UDPMux 接口
var _ ice.UDPMux = &UDPMux{}

// NewUDPMux 创建一个新的 UDP 多路复用器
// 参数:
//   - socket: net.PacketConn 底层的数据包连接
//
// 返回值:
//   - *UDPMux 新创建的 UDP 多路复用器
func NewUDPMux(socket net.PacketConn) *UDPMux {
	ctx, cancel := context.WithCancel(context.Background())
	mux := &UDPMux{
		ctx:          ctx,
		cancel:       cancel,
		socket:       socket,
		ufragMap:     make(map[ufragConnKey]*muxedConnection),
		addrMap:      make(map[string]*muxedConnection),
		ufragAddrMap: make(map[ufragConnKey][]net.Addr),
		queue:        make(chan Candidate, 32),
	}

	return mux
}

// Start 启动多路复用器的读取循环
func (mux *UDPMux) Start() {
	mux.wg.Add(1)
	go func() {
		defer mux.wg.Done()
		mux.readLoop()
	}()
}

// GetListenAddresses 实现 ice.UDPMux 接口
// 返回值:
//   - []net.Addr 监听地址列表
func (mux *UDPMux) GetListenAddresses() []net.Addr {
	return []net.Addr{mux.socket.LocalAddr()}
}

// GetConn 实现 ice.UDPMux 接口
// 如果找不到现有连接,则为给定的 ufrag 创建一个 net.PacketConn。
// 我们区分 IPv4 和 IPv6 地址,因为远程可以在同一 IP 地址族的多个不同 UDP 地址上可达(例如服务器反射地址和对等反射地址)。
//
// 参数:
//   - ufrag: string 用户片段标识符
//   - addr: net.Addr 网络地址
//
// 返回值:
//   - net.PacketConn 数据包连接
//   - error 错误信息
func (mux *UDPMux) GetConn(ufrag string, addr net.Addr) (net.PacketConn, error) {
	a, ok := addr.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("意外的地址类型: %T", addr)
	}
	select {
	case <-mux.ctx.Done():
		return nil, io.ErrClosedPipe
	default:
		isIPv6 := ok && a.IP.To4() == nil
		_, conn := mux.getOrCreateConn(ufrag, isIPv6, mux, addr)
		return conn, nil
	}
}

// Close 实现 ice.UDPMux 接口
// 关闭多路复用器
//
// 返回值:
//   - error 错误信息
func (mux *UDPMux) Close() error {
	select {
	case <-mux.ctx.Done():
		return nil
	default:
	}
	mux.cancel()
	mux.socket.Close()
	mux.wg.Wait()
	return nil
}

// writeTo 向底层的 net.PacketConn 写入数据包
//
// 参数:
//   - buf: []byte 要写入的数据
//   - addr: net.Addr 目标地址
//
// 返回值:
//   - int 写入的字节数
//   - error 错误信息
func (mux *UDPMux) writeTo(buf []byte, addr net.Addr) (int, error) {
	return mux.socket.WriteTo(buf, addr)
}

// readLoop 是多路复用器的主要读取循环
func (mux *UDPMux) readLoop() {
	for {
		select {
		case <-mux.ctx.Done():
			return
		default:
		}

		buf := pool.Get(ReceiveBufSize)

		n, addr, err := mux.socket.ReadFrom(buf)
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") || errors.Is(err, context.Canceled) {
				log.Debugf("readLoop 退出: socket %s 已关闭", mux.socket.LocalAddr())
			} else {
				log.Errorf("从 socket %s 读取时出错: %v", mux.socket.LocalAddr(), err)
			}
			pool.Put(buf)
			return
		}
		buf = buf[:n]

		if processed := mux.processPacket(buf, addr); !processed {
			pool.Put(buf)
		}
	}
}

// processPacket 处理接收到的数据包
//
// 参数:
//   - buf: []byte 数据包内容
//   - addr: net.Addr 来源地址
//
// 返回值:
//   - bool 是否成功处理
func (mux *UDPMux) processPacket(buf []byte, addr net.Addr) (processed bool) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		log.Errorf("收到非 UDP 地址: %s", addr)
		return false
	}
	isIPv6 := udpAddr.IP.To4() == nil

	// 连接通过远程地址索引。
	// 我们首先检查远程地址是否有关联的连接。
	// 如果有,我们将收到的数据包推送到该连接
	mux.mx.Lock()
	conn, ok := mux.addrMap[addr.String()]
	mux.mx.Unlock()
	if ok {
		if err := conn.Push(buf, addr); err != nil {
			log.Debugf("无法推送数据包: %v", err)
			return false
		}
		return true
	}

	if !stun.IsMessage(buf) {
		log.Debug("传入消息不是 STUN 消息")
		return false
	}

	msg := &stun.Message{Raw: buf}
	if err := msg.Decode(); err != nil {
		log.Debugf("解码 STUN 消息失败: %s", err)
		return false
	}
	if msg.Type != stun.BindingRequest {
		log.Debugf("传入消息应为 STUN 绑定请求,但收到 %s", msg.Type)
		return false
	}

	ufrag, err := ufragFromSTUNMessage(msg)
	if err != nil {
		log.Debugf("无法找到 STUN 用户名: %s", err)
		return false
	}

	connCreated, conn := mux.getOrCreateConn(ufrag, isIPv6, mux, udpAddr)
	if connCreated {
		select {
		case mux.queue <- Candidate{Addr: udpAddr, Ufrag: ufrag}:
		default:
			log.Debugw("队列已满,丢弃传入候选者", "ufrag", ufrag, "addr", udpAddr)
			conn.Close()
			return false
		}
	}

	if err := conn.Push(buf, addr); err != nil {
		log.Debugf("无法推送数据包: %v", err)
		return false
	}
	return true
}

// Accept 接受新的候选者
//
// 参数:
//   - ctx: context.Context 上下文
//
// 返回值:
//   - Candidate 候选者
//   - error 错误信息
func (mux *UDPMux) Accept(ctx context.Context) (Candidate, error) {
	select {
	case c := <-mux.queue:
		return c, nil
	case <-ctx.Done():
		return Candidate{}, ctx.Err()
	case <-mux.ctx.Done():
		return Candidate{}, mux.ctx.Err()
	}
}

// ufragConnKey 是用于索引连接的键
type ufragConnKey struct {
	ufrag  string
	isIPv6 bool
}

// ufragFromSTUNMessage 从 STUN 用户名属性返回本地或 ufrag。
// 本地 ufrag 是发起连接性检查的对等方的 ufrag,例如在从 A 到 B 的连接性检查中,用户名属性将是 B_ufrag:A_ufrag,本地 ufrag 值为 A_ufrag。
// 在 ice-lite 的情况下,localUfrag 值将始终是远程对等方的 ufrag,因为 ICE-lite 实现不生成连接性检查。
// 在我们的特定情况下,由于本地和远程 ufrag 相等,我们可以返回任一值。
//
// 参数:
//   - msg: *stun.Message STUN 消息
//
// 返回值:
//   - string ufrag 值
//   - error 错误信息
func ufragFromSTUNMessage(msg *stun.Message) (string, error) {
	attr, err := msg.Get(stun.AttrUsername)
	if err != nil {
		return "", err
	}
	index := bytes.Index(attr, []byte{':'})
	if index == -1 {
		return "", fmt.Errorf("无效的 STUN 用户名属性")
	}
	return string(attr[index+1:]), nil
}

// RemoveConnByUfrag 移除与 ufrag 关联的连接以及与该连接关联的所有地址。
// 当 peerconnection 关闭时,pion 会调用此方法。
//
// 参数:
//   - ufrag: string 要移除的连接的 ufrag
func (mux *UDPMux) RemoveConnByUfrag(ufrag string) {
	if ufrag == "" {
		return
	}

	mux.mx.Lock()
	defer mux.mx.Unlock()

	for _, isIPv6 := range [...]bool{true, false} {
		key := ufragConnKey{ufrag: ufrag, isIPv6: isIPv6}
		if _, ok := mux.ufragMap[key]; ok {
			delete(mux.ufragMap, key)
			for _, addr := range mux.ufragAddrMap[key] {
				delete(mux.addrMap, addr.String())
			}
			delete(mux.ufragAddrMap, key)
		}
	}
}

// getOrCreateConn 获取或创建与给定 ufrag 关联的连接
//
// 参数:
//   - ufrag: string 用户片段标识符
//   - isIPv6: bool 是否为 IPv6
//   - _: *UDPMux 多路复用器实例(未使用)
//   - addr: net.Addr 网络地址
//
// 返回值:
//   - bool 是否创建了新连接
//   - *muxedConnection 多路复用连接
func (mux *UDPMux) getOrCreateConn(ufrag string, isIPv6 bool, _ *UDPMux, addr net.Addr) (created bool, _ *muxedConnection) {
	key := ufragConnKey{ufrag: ufrag, isIPv6: isIPv6}

	mux.mx.Lock()
	defer mux.mx.Unlock()

	if conn, ok := mux.ufragMap[key]; ok {
		mux.addrMap[addr.String()] = conn
		mux.ufragAddrMap[key] = append(mux.ufragAddrMap[key], addr)
		return false, conn
	}

	conn := newMuxedConnection(mux, func() { mux.RemoveConnByUfrag(ufrag) })
	mux.ufragMap[key] = conn
	mux.addrMap[addr.String()] = conn
	mux.ufragAddrMap[key] = append(mux.ufragAddrMap[key], addr)
	return true, conn
}
