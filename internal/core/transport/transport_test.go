package transport

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

func TestMultiaddrParsing(t *testing.T) {
	tests := []struct {
		name     string
		addrStr  string
		wantErr  bool
		contains string
	}{
		{
			name:     "QUIC 地址",
			addrStr:  "/ip4/127.0.0.1/udp/4001/quic-v1",
			wantErr:  false,
			contains: "quic-v1",
		},
		{
			name:     "TCP 地址",
			addrStr:  "/ip4/192.168.1.1/tcp/8080",
			wantErr:  false,
			contains: "tcp",
		},
		{
			name:     "IPv6 QUIC",
			addrStr:  "/ip6/::1/udp/4001/quic-v1",
			wantErr:  false,
			contains: "ip6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := types.NewMultiaddr(tt.addrStr)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, addr)
				assert.Contains(t, addr.String(), tt.contains)
			}
		})
	}
}

func TestTransportManager_Creation(t *testing.T) {
	cfg := NewConfig()

	// NewTransportManager 需要 identity 和 upgrader，测试时传 nil
	tm := NewTransportManager(cfg, nil, nil)
	require.NotNil(t, tm)

	// 验证传输已创建
	transports := tm.GetTransports()
	assert.GreaterOrEqual(t, len(transports), 1, "至少应该有 QUIC 传输")

	// 关闭
	err := tm.Close()
	assert.NoError(t, err)
}

func TestConfig_Defaults(t *testing.T) {
	cfg := NewConfig()

	// 验证默认值
	assert.True(t, cfg.EnableQUIC, "QUIC 应默认启用")
	assert.True(t, cfg.EnableTCP, "TCP 应默认启用")
	assert.False(t, cfg.EnableWebSocket, "WebSocket 应默认禁用")
	assert.Equal(t, 2*time.Minute, cfg.QUICMaxIdleTimeout)
	assert.Equal(t, 1024, cfg.QUICMaxStreams)
}
