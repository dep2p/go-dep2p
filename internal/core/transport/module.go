// Package transport 实现传输层
package transport

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/transport/quic"
	"github.com/dep2p/go-dep2p/internal/core/transport/tcp"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"go.uber.org/fx"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/transport")

// Config 传输层配置
type Config struct {
	// 协议开关
	EnableQUIC      bool
	EnableTCP       bool
	EnableWebSocket bool

	// QUIC 配置
	QUICMaxIdleTimeout time.Duration
	QUICMaxStreams     int

	// TCP 配置
	TCPTimeout time.Duration

	// 通用配置
	DialTimeout time.Duration
}

// ConfigFromUnified 从统一配置创建传输配置
func ConfigFromUnified(cfg *config.Config) Config {
	if cfg == nil {
		return NewConfig()
	}
	return Config{
		EnableQUIC:         cfg.Transport.EnableQUIC,
		EnableTCP:          cfg.Transport.EnableTCP,
		EnableWebSocket:    cfg.Transport.EnableWebSocket,
		QUICMaxIdleTimeout: cfg.Transport.QUIC.MaxIdleTimeout.Duration(),
		QUICMaxStreams:     cfg.Transport.QUIC.MaxStreams,
		TCPTimeout:         cfg.Transport.TCP.Timeout.Duration(),
		DialTimeout:        cfg.Transport.DialTimeout.Duration(),
	}
}

// NewConfig 创建默认配置
func NewConfig() Config {
	return Config{
		EnableQUIC:      true,  // QUIC 默认启用
		EnableTCP:       true,  // TCP 默认启用
		EnableWebSocket: false, // WebSocket 默认禁用

		QUICMaxIdleTimeout: 2 * time.Minute, // 增加到 2 分钟，防止频繁断连
		QUICMaxStreams:     1024,

		TCPTimeout:  10 * time.Second,
		DialTimeout: 30 * time.Second,
	}
}

// TransportManager 传输管理器
type TransportManager struct {
	config     Config
	localPeer  types.PeerID
	identity   pkgif.Identity
	upgrader   pkgif.Upgrader
	transports []pkgif.Transport
}

// NewTransportManager 创建传输管理器
func NewTransportManager(cfg Config, identity pkgif.Identity, upgrader pkgif.Upgrader) *TransportManager {
	logger.Debug("创建传输管理器", "enableQUIC", cfg.EnableQUIC, "enableTCP", cfg.EnableTCP)
	
	localPeer := types.PeerID("")
	if identity != nil {
		localPeer = types.PeerID(identity.PeerID())
	}

	tm := &TransportManager{
		config:     cfg,
		localPeer:  localPeer,
		identity:   identity,
		upgrader:   upgrader,
		transports: make([]pkgif.Transport, 0),
	}

	// 创建 QUIC 传输（从 Identity 获取 TLS 配置）
	if cfg.EnableQUIC {
		quicTransport := quic.New(localPeer, identity)
		tm.transports = append(tm.transports, quicTransport)
		logger.Debug("QUIC 传输已创建")
	}

	// 创建 TCP 传输（需要 Upgrader 进行安全握手）
	if cfg.EnableTCP {
		tcpTransport := tcp.New(localPeer, upgrader)
		tm.transports = append(tm.transports, tcpTransport)
		logger.Debug("TCP 传输已创建")
	}

	logger.Info("传输管理器创建成功", "transportCount", len(tm.transports))
	return tm
}

// GetTransports 获取所有传输
func (tm *TransportManager) GetTransports() []pkgif.Transport {
	return tm.transports
}

// Close 关闭所有传输
func (tm *TransportManager) Close() error {
	for _, t := range tm.transports {
		t.Close()
	}
	return nil
}

// Rebind 重新绑定所有支持 Rebind 的传输
//
// 遍历所有传输，对支持 Rebind 接口的传输执行重绑定。
// 这在网络变化时（如 4G→WiFi）调用，确保 socket 使用新网络接口。
func (tm *TransportManager) Rebind(ctx context.Context) error {
	logger.Debug("重新绑定传输", "transportCount", len(tm.transports))
	
	var lastErr error
	rebindCount := 0
	
	for _, t := range tm.transports {
		// 检查传输是否支持 Rebind
		if rebinder, ok := t.(interface{ Rebind(context.Context) error }); ok {
			if err := rebinder.Rebind(ctx); err != nil {
				logger.Warn("传输重绑定失败", "error", err)
				lastErr = err
			} else {
				rebindCount++
			}
		}
	}
	
	if rebindCount > 0 {
		logger.Info("传输重绑定成功", "reboundCount", rebindCount)
		// 至少有一个传输成功 rebind
		return nil
	}
	
	logger.Warn("所有传输重绑定失败")
	return lastErr
}

// TransportOutput Fx 输出
type TransportOutput struct {
	fx.Out

	TransportManager *TransportManager
	Transports       []pkgif.Transport `group:"transports,flatten"` // 提供到 group
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("transport",
		fx.Provide(
			ProvideConfig,
			ProvideTransports,
		),
		fx.Invoke(registerLifecycle),
	)
}

// ProvideConfig 从统一配置提供传输配置
func ProvideConfig(cfg *config.Config) Config {
	return ConfigFromUnified(cfg)
}

// ProvideTransports 提供 TransportManager 和 Transport 列表
func ProvideTransports(cfg Config, identity pkgif.Identity, upgrader pkgif.Upgrader) TransportOutput {
	tm := NewTransportManager(cfg, identity, upgrader)
	return TransportOutput{
		TransportManager: tm,
		Transports:       tm.GetTransports(),
	}
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(lc fx.Lifecycle, tm *TransportManager) {
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// 传输已在创建时初始化
			return nil
		},
		OnStop: func(_ context.Context) error {
			return tm.Close()
		},
	})
}
