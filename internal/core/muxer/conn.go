package muxer

import (
	"context"

	"github.com/libp2p/go-yamux/v5"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/muxer")

// muxedConn 包装 yamux.Session，实现 MuxedConn 接口
type muxedConn struct {
	session *yamux.Session
}

// 确保实现接口
var _ pkgif.MuxedConn = (*muxedConn)(nil)

// OpenStream 打开新流
func (c *muxedConn) OpenStream(ctx context.Context) (pkgif.MuxedStream, error) {
	logger.Debug("打开多路复用流")
	s, err := c.session.OpenStream(ctx)
	if err != nil {
		logger.Warn("打开流失败", "error", err)
		return nil, parseError(err)
	}

	logger.Debug("流打开成功")
	return &muxedStream{stream: s}, nil
}

// AcceptStream 接受新流
func (c *muxedConn) AcceptStream() (pkgif.MuxedStream, error) {
	s, err := c.session.AcceptStream()
	if err != nil {
		return nil, parseError(err)
	}

	return &muxedStream{stream: s}, nil
}

// Close 关闭连接
func (c *muxedConn) Close() error {
	logger.Debug("关闭多路复用连接")
	if err := c.session.Close(); err != nil {
		logger.Warn("关闭连接失败", "error", err)
		return err
	}
	logger.Debug("连接已关闭")
	return nil
}

// IsClosed 检查连接是否已关闭
func (c *muxedConn) IsClosed() bool {
	return c.session.IsClosed()
}
