package muxer

import (
	"io"
	"math"
	"net"

	"github.com/libp2p/go-yamux/v5"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Transport yamux 多路复用器传输
type Transport struct {
	config *yamux.Config
}

// DefaultTransport 默认传输实例
var DefaultTransport *Transport

func init() {
	config := yamux.DefaultConfig()
	
	// 配置优化
	// 16MiB 窗口：100ms 延迟下可达 160MB/s 吞吐量
	config.MaxStreamWindowSize = uint32(16 * 1024 * 1024)
	
	// 禁用日志输出
	config.LogOutput = io.Discard
	
	// 禁用读缓冲（安全传输层已有缓冲）
	config.ReadBufSize = 0
	
	// 禁用入站流限制（由 ResourceManager 动态控制）
	config.MaxIncomingStreams = math.MaxUint32
	
	DefaultTransport = &Transport{config: config}
}

// NewTransport 创建新的 Transport
func NewTransport() *Transport {
	return DefaultTransport
}

// NewConn 在网络连接上创建多路复用连接
func (t *Transport) NewConn(conn net.Conn, isServer bool, scope pkgif.PeerScope) (pkgif.MuxedConn, error) {
	// 资源管理集成
	var newSpan func() (yamux.MemoryManager, error)
	if scope != nil {
		newSpan = func() (yamux.MemoryManager, error) {
			return scope.BeginSpan()
		}
	}

	var sess *yamux.Session
	var err error

	if isServer {
		sess, err = yamux.Server(conn, t.config, newSpan)
	} else {
		sess, err = yamux.Client(conn, t.config, newSpan)
	}

	if err != nil {
		return nil, err
	}

	return &muxedConn{session: sess}, nil
}

// ID 返回多路复用协议标识
func (t *Transport) ID() string {
	return "/yamux/1.0.0"
}

// Config 返回 yamux 配置（供测试使用）
func (t *Transport) Config() *yamux.Config {
	return t.config
}
