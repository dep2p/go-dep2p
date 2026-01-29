package protocol

import (
	"context"
	"io"

	"github.com/multiformats/go-multistream"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Negotiator 协议协商器
type Negotiator struct {
	registry *Registry
}

var _ pkgif.ProtocolNegotiator = (*Negotiator)(nil)

// NewNegotiator 创建协议协商器
func NewNegotiator(registry *Registry) *Negotiator {
	return &Negotiator{
		registry: registry,
	}
}

// Negotiate 协商协议（客户端模式）
// 从给定的协议列表中选择一个服务器支持的协议
func (n *Negotiator) Negotiate(_ context.Context, conn pkgif.Connection, protocols []pkgif.ProtocolID) (pkgif.ProtocolID, error) {
	// 转换为 string 数组
	protoStrs := make([]string, len(protocols))
	for i, p := range protocols {
		protoStrs[i] = string(p)
	}

	// 客户端选择
	rwc := connToReadWriteCloser(conn)
	selected, err := multistream.SelectOneOf(protoStrs, rwc)
	if err != nil {
		return "", ErrNegotiationFailed
	}

	return pkgif.ProtocolID(selected), nil
}

// Handle 处理入站协议协商（服务器模式）
// 等待客户端请求协议，返回协商的协议 ID
func (n *Negotiator) Handle(_ context.Context, conn pkgif.Connection) (pkgif.ProtocolID, error) {
	// 获取所有注册的协议
	protocols := n.registry.Protocols()

	// 创建 multistream muxer
	muxer := multistream.NewMultistreamMuxer[string]()

	// 添加所有协议
	for _, proto := range protocols {
		protoStr := string(proto)
		// 使用 nil handler，因为我们只需要协商，不需要立即处理
		muxer.AddHandler(protoStr, nil)
	}

	// 服务器端协商
	rwc := connToReadWriteCloser(conn)
	selectedProto, _, err := muxer.Negotiate(rwc)
	if err != nil {
		return "", ErrNegotiationFailed
	}

	return pkgif.ProtocolID(selectedProto), nil
}

// connToReadWriteCloser 将 Connection 转换为 io.ReadWriteCloser
func connToReadWriteCloser(conn pkgif.Connection) io.ReadWriteCloser {
	return &connAdapter{conn}
}

// connAdapter 适配器
type connAdapter struct {
	conn pkgif.Connection
}

func (a *connAdapter) Read(p []byte) (n int, err error) {
	// Connection 实现了 io.Reader
	if r, ok := a.conn.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, io.EOF
}

func (a *connAdapter) Write(p []byte) (n int, err error) {
	// Connection 实现了 io.Writer
	if w, ok := a.conn.(io.Writer); ok {
		return w.Write(p)
	}
	return 0, io.ErrClosedPipe
}

func (a *connAdapter) Close() error {
	return a.conn.Close()
}
