package noise

import (
	"bufio"
	"context"
	"net"
	"sync"
	"time"

	"github.com/flynn/noise"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// secureSession 实现了一个基于 Noise 协议的安全会话
type secureSession struct {
	initiator   bool // 是否为发起方
	checkPeerID bool // 是否检查对等点 ID

	localID   peer.ID        // 本地对等点 ID
	localKey  crypto.PrivKey // 本地私钥
	remoteID  peer.ID        // 远程对等点 ID
	remoteKey crypto.PubKey  // 远程公钥

	readLock  sync.Mutex // 读取锁
	writeLock sync.Mutex // 写入锁

	insecureConn   net.Conn      // 底层不安全连接
	insecureReader *bufio.Reader // 缓冲读取器,用于减少系统调用
	// 不缓冲写入以避免引入延迟;可能的优化点 // TODO 待重新评估

	qseek int     // 已排队字节的偏移值
	qbuf  []byte  // 已排队字节的缓冲区
	rlen  [2]byte // 用于读取传入消息长度的工作缓冲区

	enc *noise.CipherState // 加密状态
	dec *noise.CipherState // 解密状态

	// Noise 协议序言
	prologue []byte

	initiatorEarlyDataHandler, responderEarlyDataHandler EarlyDataHandler // 发起方和响应方的早期数据处理器

	// connectionState 保存与 secureSession 实体相关的状态信息
	connectionState network.ConnectionState
}

// newSecureSession 在给定的不安全连接上创建一个 Noise 会话,使用给定 Transport 的 dep2p 身份密钥对
//
// 参数:
//   - tpt: *Transport 传输层对象
//   - ctx: context.Context 上下文
//   - insecure: net.Conn 不安全的网络连接
//   - remote: peer.ID 远程对等点 ID
//   - prologue: []byte 序言数据
//   - initiatorEDH: EarlyDataHandler 发起方早期数据处理器
//   - responderEDH: EarlyDataHandler 响应方早期数据处理器
//   - initiator: bool 是否为发起方
//   - checkPeerID: bool 是否检查对等点 ID
//
// 返回值:
//   - *secureSession 安全会话对象
//   - error 创建过程中的错误
func newSecureSession(tpt *Transport, ctx context.Context, insecure net.Conn, remote peer.ID, prologue []byte, initiatorEDH, responderEDH EarlyDataHandler, initiator, checkPeerID bool) (*secureSession, error) {
	// 初始化安全会话对象
	s := &secureSession{
		insecureConn:              insecure,
		insecureReader:            bufio.NewReader(insecure),
		initiator:                 initiator,
		localID:                   tpt.localID,
		localKey:                  tpt.privateKey,
		remoteID:                  remote,
		prologue:                  prologue,
		initiatorEarlyDataHandler: initiatorEDH,
		responderEarlyDataHandler: responderEDH,
		checkPeerID:               checkPeerID,
	}

	// 创建一个 goroutine 运行握手,并将握手结果写入 respCh
	respCh := make(chan error, 1)
	go func() {
		respCh <- s.runHandshake(ctx)
	}()

	select {
	case err := <-respCh:
		if err != nil {
			log.Debugf("握手过程中出错: %s", err)
			_ = s.insecureConn.Close()
		}
		return s, err

	case <-ctx.Done():
		// 如果上下文被取消,关闭底层连接
		// 等待握手返回第一个错误,以确保在清理 goroutine 前不返回
		_ = s.insecureConn.Close()
		<-respCh
		return nil, ctx.Err()
	}
}

// LocalAddr 返回本地网络地址
//
// 返回值:
//   - net.Addr 本地网络地址
func (s *secureSession) LocalAddr() net.Addr {
	return s.insecureConn.LocalAddr()
}

// LocalPeer 返回本地对等点 ID
//
// 返回值:
//   - peer.ID 本地对等点 ID
func (s *secureSession) LocalPeer() peer.ID {
	return s.localID
}

// LocalPublicKey 返回本地公钥
//
// 返回值:
//   - crypto.PubKey 本地公钥
func (s *secureSession) LocalPublicKey() crypto.PubKey {
	return s.localKey.GetPublic()
}

// RemoteAddr 返回远程网络地址
//
// 返回值:
//   - net.Addr 远程网络地址
func (s *secureSession) RemoteAddr() net.Addr {
	return s.insecureConn.RemoteAddr()
}

// RemotePeer 返回远程对等点 ID
//
// 返回值:
//   - peer.ID 远程对等点 ID
func (s *secureSession) RemotePeer() peer.ID {
	return s.remoteID
}

// RemotePublicKey 返回远程公钥
//
// 返回值:
//   - crypto.PubKey 远程公钥
func (s *secureSession) RemotePublicKey() crypto.PubKey {
	return s.remoteKey
}

// ConnState 返回连接状态
//
// 返回值:
//   - network.ConnectionState 连接状态
func (s *secureSession) ConnState() network.ConnectionState {
	return s.connectionState
}

// SetDeadline 设置读写操作的截止时间
//
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error 设置过程中的错误
func (s *secureSession) SetDeadline(t time.Time) error {
	return s.insecureConn.SetDeadline(t)
}

// SetReadDeadline 设置读操作的截止时间
//
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error 设置过程中的错误
func (s *secureSession) SetReadDeadline(t time.Time) error {
	return s.insecureConn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写操作的截止时间
//
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error 设置过程中的错误
func (s *secureSession) SetWriteDeadline(t time.Time) error {
	return s.insecureConn.SetWriteDeadline(t)
}

// Close 关闭安全会话
//
// 返回值:
//   - error 关闭过程中的错误
func (s *secureSession) Close() error {
	return s.insecureConn.Close()
}

// SessionWithConnState 设置会话的连接状态并返回会话对象
//
// 参数:
//   - s: *secureSession 安全会话对象
//   - muxer: protocol.ID 多路复用器协议 ID
//
// 返回值:
//   - *secureSession 更新后的安全会话对象
func SessionWithConnState(s *secureSession, muxer protocol.ID) *secureSession {
	if s != nil {
		s.connectionState.StreamMultiplexer = muxer
		s.connectionState.UsedEarlyMuxerNegotiation = muxer != ""
	}
	return s
}
