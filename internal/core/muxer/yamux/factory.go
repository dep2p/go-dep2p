// Package yamux 提供基于 yamux 的多路复用实现
package yamux

import (
	"fmt"
	"io"

	"github.com/hashicorp/yamux"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

// Factory 实现 muxer.MuxerFactory 接口
// 用于创建 yamux 多路复用器
type Factory struct {
	config     muxerif.Config
	yamuxCfg   *yamux.Config
}

// 确保实现 muxer.MuxerFactory 接口
var _ muxerif.MuxerFactory = (*Factory)(nil)

// NewFactory 创建 yamux 工厂
func NewFactory(config muxerif.Config) *Factory {
	return &Factory{
		config:   config,
		yamuxCfg: ConfigToYamux(config),
	}
}

// NewFactoryWithYamuxConfig 使用 yamux 原生配置创建工厂
func NewFactoryWithYamuxConfig(yamuxCfg *yamux.Config) *Factory {
	return &Factory{
		config:   YamuxToConfig(yamuxCfg),
		yamuxCfg: yamuxCfg,
	}
}

// NewMuxer 从连接创建多路复用器
func (f *Factory) NewMuxer(conn io.ReadWriteCloser, isServer bool) (muxerif.Muxer, error) {
	if conn == nil {
		return nil, fmt.Errorf("连接不能为 nil")
	}

	var session *yamux.Session
	var err error

	if isServer {
		session, err = yamux.Server(conn, f.yamuxCfg)
	} else {
		session, err = yamux.Client(conn, f.yamuxCfg)
	}

	if err != nil {
		return nil, fmt.Errorf("创建 yamux session 失败: %w", err)
	}

	return NewMuxer(session, isServer), nil
}

// Protocol 返回协议名称
func (f *Factory) Protocol() string {
	return "yamux"
}

// Config 返回配置
func (f *Factory) Config() muxerif.Config {
	return f.config
}

// YamuxConfig 返回 yamux 原生配置
func (f *Factory) YamuxConfig() *yamux.Config {
	return f.yamuxCfg
}

