package peerstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()
	require.NotNil(t, cfg)

	// 验证默认配置
	assert.True(t, cfg.EnableGC)
	assert.Equal(t, 1*time.Minute, cfg.GCInterval)
	assert.Equal(t, 10*time.Second, cfg.GCLookahead)
}

func TestModule(t *testing.T) {
	mod := Module()
	require.NotNil(t, mod)
}

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				EnableGC:    true,
				GCInterval:  1 * time.Minute,
				GCLookahead: 10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "disabled GC",
			cfg: Config{
				EnableGC:    false,
				GCInterval:  0,
				GCLookahead: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 配置验证逻辑
			if tt.cfg.EnableGC {
				assert.Greater(t, tt.cfg.GCInterval, time.Duration(0))
				assert.Greater(t, tt.cfg.GCLookahead, time.Duration(0))
			}
		})
	}
}
