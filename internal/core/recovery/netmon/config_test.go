// Package netmon 网络状态监控 - 配置测试
package netmon

import (
	"testing"
	"time"
)

// ============================================================================
//                              T19: 配置测试
// ============================================================================

// TestConfig_Default 测试默认配置
func TestConfig_Default(t *testing.T) {
	config := DefaultConfig()

	if config.ErrorThreshold != 3 {
		t.Errorf("expected ErrorThreshold=3, got %d", config.ErrorThreshold)
	}

	if config.ProbeInterval != 30*time.Second {
		t.Errorf("expected ProbeInterval=30s, got %v", config.ProbeInterval)
	}

	if config.RecoveryProbeInterval != 1*time.Second {
		t.Errorf("expected RecoveryProbeInterval=1s, got %v", config.RecoveryProbeInterval)
	}

	if config.ErrorWindow != 1*time.Minute {
		t.Errorf("expected ErrorWindow=1m, got %v", config.ErrorWindow)
	}

	if config.MaxRecoveryAttempts != 5 {
		t.Errorf("expected MaxRecoveryAttempts=5, got %d", config.MaxRecoveryAttempts)
	}

	if config.StateChangeDebounce != 500*time.Millisecond {
		t.Errorf("expected StateChangeDebounce=500ms, got %v", config.StateChangeDebounce)
	}

	if !config.EnableAutoRecovery {
		t.Error("expected EnableAutoRecovery=true")
	}

	if len(config.CriticalErrors) == 0 {
		t.Error("expected CriticalErrors to be non-empty")
	}
}

// TestConfig_Validate 测试配置验证
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		check  func(*testing.T, *Config)
	}{
		{
			name: "零值ErrorThreshold修正为默认值",
			config: &Config{
				ErrorThreshold: 0,
			},
			check: func(t *testing.T, c *Config) {
				if c.ErrorThreshold != 3 {
					t.Errorf("expected ErrorThreshold=3, got %d", c.ErrorThreshold)
				}
			},
		},
		{
			name: "负值ErrorThreshold修正为默认值",
			config: &Config{
				ErrorThreshold: -5,
			},
			check: func(t *testing.T, c *Config) {
				if c.ErrorThreshold != 3 {
					t.Errorf("expected ErrorThreshold=3, got %d", c.ErrorThreshold)
				}
			},
		},
		{
			name: "零值ProbeInterval修正为默认值",
			config: &Config{
				ProbeInterval: 0,
			},
			check: func(t *testing.T, c *Config) {
				if c.ProbeInterval != 30*time.Second {
					t.Errorf("expected ProbeInterval=30s, got %v", c.ProbeInterval)
				}
			},
		},
		{
			name: "空CriticalErrors修正为默认值",
			config: &Config{
				CriticalErrors: []string{},
			},
			check: func(t *testing.T, c *Config) {
				if len(c.CriticalErrors) == 0 {
					t.Error("expected CriticalErrors to be populated")
				}
			},
		},
		{
			name: "零值BackoffFactor修正为默认值",
			config: &Config{
				BackoffFactor: 0,
			},
			check: func(t *testing.T, c *Config) {
				if c.BackoffFactor != 1.5 {
					t.Errorf("expected BackoffFactor=1.5, got %f", c.BackoffFactor)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != nil {
				t.Errorf("Validate() returned error: %v", err)
			}
			tt.check(t, tt.config)
		})
	}
}

// TestConfig_WithMethods 测试 With* 配置方法
func TestConfig_WithMethods(t *testing.T) {
	config := DefaultConfig()

	// WithErrorThreshold
	config.WithErrorThreshold(10)
	if config.ErrorThreshold != 10 {
		t.Errorf("WithErrorThreshold failed: expected 10, got %d", config.ErrorThreshold)
	}

	// WithProbeInterval
	config.WithProbeInterval(5 * time.Second)
	if config.ProbeInterval != 5*time.Second {
		t.Errorf("WithProbeInterval failed: expected 5s, got %v", config.ProbeInterval)
	}

	// WithCriticalErrors
	customErrors := []string{"custom error 1", "custom error 2"}
	config.WithCriticalErrors(customErrors)
	if len(config.CriticalErrors) != 2 {
		t.Errorf("WithCriticalErrors failed: expected 2 errors, got %d", len(config.CriticalErrors))
	}

	// WithAutoRecovery
	config.WithAutoRecovery(false)
	if config.EnableAutoRecovery {
		t.Error("WithAutoRecovery failed: expected false")
	}

	// WithStateChangeDebounce
	config.WithStateChangeDebounce(100 * time.Millisecond)
	if config.StateChangeDebounce != 100*time.Millisecond {
		t.Errorf("WithStateChangeDebounce failed: expected 100ms, got %v", config.StateChangeDebounce)
	}
}

// TestConfig_Chaining 测试配置链式调用
func TestConfig_Chaining(t *testing.T) {
	config := DefaultConfig().
		WithErrorThreshold(5).
		WithProbeInterval(10 * time.Second).
		WithAutoRecovery(false).
		WithStateChangeDebounce(200 * time.Millisecond)

	if config.ErrorThreshold != 5 {
		t.Errorf("expected ErrorThreshold=5, got %d", config.ErrorThreshold)
	}
	if config.ProbeInterval != 10*time.Second {
		t.Errorf("expected ProbeInterval=10s, got %v", config.ProbeInterval)
	}
	if config.EnableAutoRecovery {
		t.Error("expected EnableAutoRecovery=false")
	}
	if config.StateChangeDebounce != 200*time.Millisecond {
		t.Errorf("expected StateChangeDebounce=200ms, got %v", config.StateChangeDebounce)
	}
}

// TestConfig_ToInterfaceConfig 测试转换为接口配置
func TestConfig_ToInterfaceConfig(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 10
	config.ProbeInterval = 5 * time.Second

	ifaceConfig := config.ToInterfaceConfig()

	if ifaceConfig.ErrorThreshold != 10 {
		t.Errorf("expected ErrorThreshold=10, got %d", ifaceConfig.ErrorThreshold)
	}
	if ifaceConfig.ProbeInterval != 5*time.Second {
		t.Errorf("expected ProbeInterval=5s, got %v", ifaceConfig.ProbeInterval)
	}
	if len(ifaceConfig.CriticalErrors) == 0 {
		t.Error("expected CriticalErrors to be copied")
	}
}

// TestConfig_FromInterfaceConfig 测试从接口配置创建
func TestConfig_FromInterfaceConfig(t *testing.T) {
	original := DefaultConfig()
	ifaceConfig := original.ToInterfaceConfig()

	// 修改接口配置
	ifaceConfig.ErrorThreshold = 15
	ifaceConfig.ProbeInterval = 3 * time.Second

	// 从接口配置创建
	config := FromInterfaceConfig(ifaceConfig)

	if config.ErrorThreshold != 15 {
		t.Errorf("expected ErrorThreshold=15, got %d", config.ErrorThreshold)
	}
	if config.ProbeInterval != 3*time.Second {
		t.Errorf("expected ProbeInterval=3s, got %v", config.ProbeInterval)
	}
}

// TestConfig_EdgeCases 测试配置边界情况
func TestConfig_EdgeCases(t *testing.T) {
	t.Run("极小值", func(t *testing.T) {
		config := &Config{
			ErrorThreshold:        -100,
			ProbeInterval:         -1 * time.Second,
			RecoveryProbeInterval: -1 * time.Second,
			ErrorWindow:           -1 * time.Minute,
			MaxRecoveryAttempts:   -10,
			InitialBackoff:        -1 * time.Second,
			MaxBackoff:            -1 * time.Second,
			BackoffFactor:         -1.0,
			StateChangeDebounce:   -1 * time.Millisecond,
		}

		config.Validate()

		// 所有负值应该被修正为默认值
		if config.ErrorThreshold <= 0 {
			t.Error("ErrorThreshold not corrected")
		}
		if config.ProbeInterval <= 0 {
			t.Error("ProbeInterval not corrected")
		}
		if config.RecoveryProbeInterval <= 0 {
			t.Error("RecoveryProbeInterval not corrected")
		}
		if config.ErrorWindow <= 0 {
			t.Error("ErrorWindow not corrected")
		}
		if config.MaxRecoveryAttempts <= 0 {
			t.Error("MaxRecoveryAttempts not corrected")
		}
		if config.InitialBackoff <= 0 {
			t.Error("InitialBackoff not corrected")
		}
		if config.MaxBackoff <= 0 {
			t.Error("MaxBackoff not corrected")
		}
		if config.BackoffFactor <= 1.0 {
			t.Error("BackoffFactor not corrected")
		}
		if config.StateChangeDebounce <= 0 {
			t.Error("StateChangeDebounce not corrected")
		}
	})

	t.Run("极大值", func(t *testing.T) {
		config := &Config{
			ErrorThreshold:        10000,
			ProbeInterval:         24 * time.Hour,
			RecoveryProbeInterval: 1 * time.Hour,
			ErrorWindow:           24 * time.Hour,
			MaxRecoveryAttempts:   1000,
			InitialBackoff:        10 * time.Minute,
			MaxBackoff:            10 * time.Hour,
			BackoffFactor:         100.0,
			StateChangeDebounce:   1 * time.Hour,
		}

		config.Validate()

		// 极大值应该保持不变（只要大于0）
		if config.ErrorThreshold != 10000 {
			t.Error("ErrorThreshold was modified")
		}
		if config.ProbeInterval != 24*time.Hour {
			t.Error("ProbeInterval was modified")
		}
	})
}
