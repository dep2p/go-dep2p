package protocol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestDefaultConfig 测试默认配置
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	
	assert.Equal(t, 10*time.Second, cfg.NegotiationTimeout)
	
	t.Log("✅ DefaultConfig 正确")
}

// TestConfig_Validate 测试配置验证
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "有效配置",
			cfg: Config{
				NegotiationTimeout: 10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "超时为 0",
			cfg: Config{
				NegotiationTimeout: 0,
			},
			wantErr: true,
		},
		{
			name: "超时为负数",
			cfg: Config{
				NegotiationTimeout: -1 * time.Second,
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
	
	t.Log("✅ Config Validate 正确")
}

// TestConfig_WithNegotiationTimeout 测试设置超时
func TestConfig_WithNegotiationTimeout(t *testing.T) {
	cfg := DefaultConfig()
	
	newCfg := cfg.WithNegotiationTimeout(20 * time.Second)
	assert.Equal(t, 20*time.Second, newCfg.NegotiationTimeout)
	
	t.Log("✅ WithNegotiationTimeout 正确")
}
