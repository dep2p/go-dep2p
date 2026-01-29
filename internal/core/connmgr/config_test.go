package connmgr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultConfig 测试默认配置
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 100, cfg.LowWater)
	assert.Equal(t, 400, cfg.HighWater)
	assert.Equal(t, 20*time.Second, cfg.GracePeriod)

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
				LowWater:  100,
				HighWater: 400,
			},
			wantErr: false,
		},
		{
			name: "LowWater 为 0",
			cfg: Config{
				LowWater:  0,
				HighWater: 400,
			},
			wantErr: true,
		},
		{
			name: "LowWater 为负数",
			cfg: Config{
				LowWater:  -1,
				HighWater: 400,
			},
			wantErr: true,
		},
		{
			name: "HighWater 等于 LowWater",
			cfg: Config{
				LowWater:  100,
				HighWater: 100,
			},
			wantErr: true,
		},
		{
			name: "HighWater 小于 LowWater",
			cfg: Config{
				LowWater:  400,
				HighWater: 100,
			},
			wantErr: true,
		},
		{
			name: "GracePeriod 为负数",
			cfg: Config{
				LowWater:    100,
				HighWater:   400,
				GracePeriod: -1 * time.Second,
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

// TestConfig_WithLowWater 测试设置低水位
func TestConfig_WithLowWater(t *testing.T) {
	cfg := DefaultConfig()

	newCfg := cfg.WithLowWater(200)
	assert.Equal(t, 200, newCfg.LowWater)
	assert.Equal(t, cfg.HighWater, newCfg.HighWater) // 其他字段不变

	t.Log("✅ Config WithLowWater 正确")
}

// TestConfig_WithHighWater 测试设置高水位
func TestConfig_WithHighWater(t *testing.T) {
	cfg := DefaultConfig()

	newCfg := cfg.WithHighWater(800)
	assert.Equal(t, 800, newCfg.HighWater)
	assert.Equal(t, cfg.LowWater, newCfg.LowWater) // 其他字段不变

	t.Log("✅ Config WithHighWater 正确")
}

// TestConfig_WithGracePeriod 测试设置保护期
func TestConfig_WithGracePeriod(t *testing.T) {
	cfg := DefaultConfig()

	newCfg := cfg.WithGracePeriod(30 * time.Second)
	assert.Equal(t, 30*time.Second, newCfg.GracePeriod)
	assert.Equal(t, cfg.LowWater, newCfg.LowWater) // 其他字段不变

	t.Log("✅ Config WithGracePeriod 正确")
}

// TestNew_WithInvalidConfig 测试无效配置
func TestNew_WithInvalidConfig(t *testing.T) {
	cfg := Config{
		LowWater:  0,
		HighWater: 400,
	}

	_, err := New(cfg)
	require.Error(t, err)
	assert.Equal(t, ErrInvalidConfig, err)

	t.Log("✅ New 拒绝无效配置")
}
