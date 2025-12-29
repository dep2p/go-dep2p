// Package muxer 提供多路复用模块的实现
package muxer

import (
	"io"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

// CompositeFactory 复合 muxer 工厂
// 根据连接类型选择合适的 muxer：
// - QUIC 连接：使用 QUIC 内置多路复用的适配器
// - 其他连接：使用 yamux
type CompositeFactory struct {
	quicFactory    muxerif.MuxerFactory // 通过接口注入，不直接依赖 transport/quic 实现
	defaultFactory muxerif.MuxerFactory
}

// NewCompositeFactory 创建复合 muxer 工厂
//
// 参数：
//   - quicFactory: QUIC 连接的 muxer 工厂（可为 nil，则 QUIC 连接回退到 defaultFactory）
//   - defaultFactory: 默认 muxer 工厂（如 yamux）
func NewCompositeFactory(quicFactory, defaultFactory muxerif.MuxerFactory) *CompositeFactory {
	return &CompositeFactory{
		quicFactory:    quicFactory,
		defaultFactory: defaultFactory,
	}
}

// NewMuxer 从连接创建多路复用器
func (f *CompositeFactory) NewMuxer(conn io.ReadWriteCloser, isServer bool) (muxerif.Muxer, error) {
	// 检查是否是 QUIC 连接，并且有 QUIC 工厂
	if f.quicFactory != nil && isQUICConn(conn) {
		muxer, err := f.quicFactory.NewMuxer(conn, isServer)
		if err == nil {
			return muxer, nil
		}
		// 如果 QUIC 适配器失败，回退到默认工厂
	}

	// 使用默认工厂（yamux）
	return f.defaultFactory.NewMuxer(conn, isServer)
}

// Protocol 返回协议名称
func (f *CompositeFactory) Protocol() string {
	return "composite"
}

// isQUICConn 检查连接是否是 QUIC 连接
//
// 仅使用通用能力接口判断，不依赖具体实现类型。
func isQUICConn(conn io.ReadWriteCloser) bool {
	// 检查 Transport 方法是否返回 "quic"
	if tc, ok := conn.(interface{ Transport() string }); ok {
		return tc.Transport() == "quic"
	}

	// 检查是否暴露 QuicConn 方法（通用能力接口）
	if _, ok := conn.(interface{ QuicConn() interface{} }); ok {
		return true
	}

	return false
}
