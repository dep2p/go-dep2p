package security

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Config 测试
// ============================================================================

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()
	require.NotNil(t, cfg)

	// 验证默认配置
	t.Run("默认启用TLS和Noise", func(t *testing.T) {
		assert.Contains(t, cfg.Transports, "tls")
		assert.Contains(t, cfg.Transports, "noise")
		assert.Len(t, cfg.Transports, 2)
	})

	t.Run("默认首选TLS", func(t *testing.T) {
		assert.Equal(t, "tls", cfg.Preferred)
	})

	t.Run("默认超时60秒", func(t *testing.T) {
		assert.Equal(t, defaultNegotiateTimeout, cfg.NegotiateTimeout)
		assert.Equal(t, 60*time.Second, cfg.NegotiateTimeout)
	})
}

func TestConfigFromUnified(t *testing.T) {
	t.Run("nil配置返回默认值", func(t *testing.T) {
		cfg := ConfigFromUnified(nil)
		assert.Equal(t, NewConfig(), cfg)
	})

	t.Run("仅启用TLS", func(t *testing.T) {
		ucfg := &config.Config{
			Security: config.SecurityConfig{
				EnableTLS:   true,
				EnableNoise: false,
			},
		}
		cfg := ConfigFromUnified(ucfg)
		assert.Contains(t, cfg.Transports, "tls")
		assert.NotContains(t, cfg.Transports, "noise")
	})

	t.Run("仅启用Noise", func(t *testing.T) {
		ucfg := &config.Config{
			Security: config.SecurityConfig{
				EnableTLS:   false,
				EnableNoise: true,
			},
		}
		cfg := ConfigFromUnified(ucfg)
		assert.NotContains(t, cfg.Transports, "tls")
		assert.Contains(t, cfg.Transports, "noise")
	})

	t.Run("都不启用时默认启用TLS", func(t *testing.T) {
		ucfg := &config.Config{
			Security: config.SecurityConfig{
				EnableTLS:   false,
				EnableNoise: false,
			},
		}
		cfg := ConfigFromUnified(ucfg)
		assert.Contains(t, cfg.Transports, "tls")
		assert.Len(t, cfg.Transports, 1)
	})
}

// ============================================================================
// SecurityMux 测试
// ============================================================================

func TestNewSecurityMux(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	t.Run("正常创建", func(t *testing.T) {
		cfg := NewConfig()
		params := SecurityMuxParams{
			Identity: id,
			Config:   cfg,
		}
		mux, err := NewSecurityMux(params)
		require.NoError(t, err)
		require.NotNil(t, mux)

		// 验证协议列表
		protocols := mux.ListProtocols()
		assert.Contains(t, protocols, "tls")
		assert.Contains(t, protocols, "noise")
	})

	t.Run("nil身份应失败", func(t *testing.T) {
		cfg := NewConfig()
		params := SecurityMuxParams{
			Identity: nil,
			Config:   cfg,
		}
		_, err := NewSecurityMux(params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "identity is nil")
	})

	t.Run("未知协议应失败", func(t *testing.T) {
		cfg := Config{
			Transports: []string{"unknown"},
			Preferred:  "unknown",
		}
		params := SecurityMuxParams{
			Identity: id,
			Config:   cfg,
		}
		_, err := NewSecurityMux(params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown transport protocol")
	})

	t.Run("空协议列表应失败", func(t *testing.T) {
		cfg := Config{
			Transports: []string{},
		}
		params := SecurityMuxParams{
			Identity: id,
			Config:   cfg,
		}
		_, err := NewSecurityMux(params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no transport protocols enabled")
	})

	t.Run("首选协议不存在应失败", func(t *testing.T) {
		cfg := Config{
			Transports: []string{"tls"},
			Preferred:  "noise", // 不在 Transports 中
		}
		params := SecurityMuxParams{
			Identity: id,
			Config:   cfg,
		}
		_, err := NewSecurityMux(params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "preferred protocol noise not enabled")
	})

	t.Run("未指定首选时使用第一个", func(t *testing.T) {
		cfg := Config{
			Transports: []string{"noise"},
			Preferred:  "",
		}
		params := SecurityMuxParams{
			Identity: id,
			Config:   cfg,
		}
		mux, err := NewSecurityMux(params)
		require.NoError(t, err)
		assert.NotEmpty(t, mux.preferred)
	})
}

func TestSecurityMux_ID(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	mux, err := NewSecurityMux(SecurityMuxParams{
		Identity: id,
		Config:   NewConfig(),
	})
	require.NoError(t, err)

	assert.Equal(t, types.ProtocolID("/security/multistream/1.0.0"), mux.ID())
}

func TestSecurityMux_ListProtocols(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	t.Run("仅TLS", func(t *testing.T) {
		cfg := Config{
			Transports: []string{"tls"},
			Preferred:  "tls",
		}
		mux, err := NewSecurityMux(SecurityMuxParams{
			Identity: id,
			Config:   cfg,
		})
		require.NoError(t, err)

		protocols := mux.ListProtocols()
		assert.Len(t, protocols, 1)
		assert.Contains(t, protocols, "tls")
	})

	t.Run("TLS和Noise", func(t *testing.T) {
		mux, err := NewSecurityMux(SecurityMuxParams{
			Identity: id,
			Config:   NewConfig(),
		})
		require.NoError(t, err)

		protocols := mux.ListProtocols()
		assert.Len(t, protocols, 2)
	})
}

func TestSecurityMux_SetPreferred(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	mux, err := NewSecurityMux(SecurityMuxParams{
		Identity: id,
		Config:   NewConfig(),
	})
	require.NoError(t, err)

	t.Run("设置已存在的协议", func(t *testing.T) {
		err := mux.SetPreferred("noise")
		assert.NoError(t, err)
	})

	t.Run("设置不存在的协议应失败", func(t *testing.T) {
		err := mux.SetPreferred("unknown")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "protocol unknown not enabled")
	})
}

func TestSecurityMux_Handshake(t *testing.T) {
	// 创建两个身份
	serverIdentity, err := identity.Generate()
	require.NoError(t, err)
	clientIdentity, err := identity.Generate()
	require.NoError(t, err)

	serverPeer := types.PeerID(serverIdentity.PeerID())
	clientPeer := types.PeerID(clientIdentity.PeerID())

	// 创建 SecurityMux
	serverMux, err := NewSecurityMux(SecurityMuxParams{
		Identity: serverIdentity,
		Config:   NewConfig(),
	})
	require.NoError(t, err)

	clientMux, err := NewSecurityMux(SecurityMuxParams{
		Identity: clientIdentity,
		Config:   NewConfig(),
	})
	require.NoError(t, err)

	// 创建管道连接
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	errCh := make(chan error, 2)
	doneCh := make(chan struct{}, 2)

	// 服务器握手
	go func() {
		secConn, err := serverMux.SecureInbound(ctx, serverConn, clientPeer)
		if err != nil {
			errCh <- err
			return
		}
		defer secConn.Close()
		assert.Equal(t, serverPeer, secConn.LocalPeer())
		assert.Equal(t, clientPeer, secConn.RemotePeer())
		doneCh <- struct{}{}
	}()

	// 客户端握手
	go func() {
		secConn, err := clientMux.SecureOutbound(ctx, clientConn, serverPeer)
		if err != nil {
			errCh <- err
			return
		}
		defer secConn.Close()
		assert.Equal(t, clientPeer, secConn.LocalPeer())
		assert.Equal(t, serverPeer, secConn.RemotePeer())
		doneCh <- struct{}{}
	}()

	// 等待完成
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			t.Fatalf("握手失败: %v", err)
		case <-doneCh:
			// OK
		case <-ctx.Done():
			t.Fatal("握手超时")
		}
	}
}

func TestModule(t *testing.T) {
	// Module() 返回 Fx 模块定义
	// 这里只验证它返回非空值
	opt := Module()
	assert.NotNil(t, opt)
}
