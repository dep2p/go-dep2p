package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewConfig 测试创建默认配置
func TestNewConfig(t *testing.T) {
	cfg := NewConfig()
	require.NotNil(t, cfg)

	// 验证默认配置有效
	err := cfg.Validate()
	assert.NoError(t, err)

	t.Log("✅ NewConfig 测试通过")
}

// TestConfig_Validate 测试配置验证
func TestConfig_Validate(t *testing.T) {
	cfg := NewConfig()

	err := cfg.Validate()
	assert.NoError(t, err)

	t.Log("✅ Config.Validate 测试通过")
}

// TestIdentityConfig 测试身份配置
func TestIdentityConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultIdentityConfig()
		assert.Equal(t, "Ed25519", cfg.KeyType)
		assert.True(t, cfg.AutoGenerate)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultIdentityConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("Validate_InvalidKeyType", func(t *testing.T) {
		cfg := DefaultIdentityConfig()
		cfg.KeyType = "invalid"
		err := cfg.Validate()
		assert.Error(t, err)
	})

	t.Run("WithKeyType", func(t *testing.T) {
		cfg := DefaultIdentityConfig().WithKeyType("RSA")
		assert.Equal(t, "RSA", cfg.KeyType)
	})

	t.Log("✅ IdentityConfig 测试通过")
}

// TestTransportConfig 测试传输配置
func TestTransportConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultTransportConfig()
		assert.True(t, cfg.EnableQUIC)
		assert.True(t, cfg.EnableTCP)
		assert.False(t, cfg.EnableWebSocket)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultTransportConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("Validate_NoTransport", func(t *testing.T) {
		cfg := DefaultTransportConfig()
		cfg.EnableQUIC = false
		cfg.EnableTCP = false
		cfg.EnableWebSocket = false
		err := cfg.Validate()
		assert.Error(t, err)
	})

	t.Run("WithQUIC", func(t *testing.T) {
		cfg := DefaultTransportConfig().WithQUIC(false)
		assert.False(t, cfg.EnableQUIC)
	})

	t.Log("✅ TransportConfig 测试通过")
}

// TestSecurityConfig 测试安全配置
func TestSecurityConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultSecurityConfig()
		assert.True(t, cfg.EnableTLS)
		assert.True(t, cfg.EnableNoise)
		assert.Equal(t, "tls", cfg.PreferredProtocol)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultSecurityConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("Validate_NoSecurity", func(t *testing.T) {
		cfg := DefaultSecurityConfig()
		cfg.EnableTLS = false
		cfg.EnableNoise = false
		err := cfg.Validate()
		assert.Error(t, err)
	})

	t.Log("✅ SecurityConfig 测试通过")
}

// TestNATConfig 测试 NAT 配置
func TestNATConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultNATConfig()
		assert.True(t, cfg.EnableAutoNAT)
		assert.True(t, cfg.EnableUPnP)
		assert.True(t, cfg.EnableNATPMP)
		assert.True(t, cfg.EnableHolePunch)
		assert.NotEmpty(t, cfg.STUNServers)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultNATConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Log("✅ NATConfig 测试通过")
}

// TestRelayConfig 测试中继配置
func TestRelayConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultRelayConfig()
		assert.True(t, cfg.EnableClient)
		assert.False(t, cfg.EnableServer)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultRelayConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Log("✅ RelayConfig 测试通过")
}

// TestDiscoveryConfig 测试发现配置
func TestDiscoveryConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultDiscoveryConfig()
		assert.True(t, cfg.EnableDHT)
		// mDNS 默认启用：支持局域网自动发现（支持优雅降级）
		assert.True(t, cfg.EnableMDNS)
		assert.True(t, cfg.EnableBootstrap)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultDiscoveryConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("Validate_NoDiscovery", func(t *testing.T) {
		cfg := DefaultDiscoveryConfig()
		cfg.EnableDHT = false
		cfg.EnableMDNS = false
		cfg.EnableBootstrap = false
		cfg.EnableRendezvous = false
		cfg.EnableDNS = false
		err := cfg.Validate()
		assert.Error(t, err)
	})

	t.Log("✅ DiscoveryConfig 测试通过")
}

// TestConnManagerConfig 测试连接管理配置
func TestConnManagerConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultConnManagerConfig()
		assert.Equal(t, 100, cfg.LowWater)
		assert.Equal(t, 400, cfg.HighWater)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultConnManagerConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("Validate_LowGreaterThanHigh", func(t *testing.T) {
		cfg := DefaultConnManagerConfig()
		cfg.LowWater = 500
		cfg.HighWater = 100
		err := cfg.Validate()
		assert.Error(t, err)
	})

	t.Log("✅ ConnManagerConfig 测试通过")
}

// TestMessagingConfig 测试消息传递配置
func TestMessagingConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultMessagingConfig()
		// 消息服务默认禁用：需要 Realm 支持，由 Realm 工厂管理
		assert.False(t, cfg.EnablePubSub)
		assert.False(t, cfg.EnableStreams)
		assert.False(t, cfg.EnableLiveness)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultMessagingConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Log("✅ MessagingConfig 测试通过")
}

// TestRealmConfig 测试 Realm 配置
func TestRealmConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultRealmConfig()
		// Realm 组件默认禁用：由 Realm 工厂在创建 Realm 时内部管理
		assert.False(t, cfg.EnableGateway)
		assert.False(t, cfg.EnableRouting)
		assert.False(t, cfg.EnableMember)
		assert.False(t, cfg.EnableAuth)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultRealmConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Log("✅ RealmConfig 测试通过")
}

// TestResourceConfig 测试资源配置
func TestResourceConfig(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		cfg := DefaultResourceConfig()
		assert.True(t, cfg.EnableResourceManager)
		assert.Equal(t, 1000, cfg.System.MaxConnections)
	})

	t.Run("Validate_Valid", func(t *testing.T) {
		cfg := DefaultResourceConfig()
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Log("✅ ResourceConfig 测试通过")
}

// TestPresetConfigs 测试预设配置
func TestPresetConfigs(t *testing.T) {
	t.Run("MobileConfig", func(t *testing.T) {
		cfg := NewMobileConfig()
		require.NotNil(t, cfg)
		assert.True(t, cfg.Transport.EnableQUIC)
		assert.False(t, cfg.Transport.EnableTCP)
		assert.LessOrEqual(t, cfg.ConnMgr.HighWater, 100)
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("ServerConfig", func(t *testing.T) {
		cfg := NewServerConfig()
		require.NotNil(t, cfg)
		assert.True(t, cfg.Relay.EnableServer)
		assert.True(t, cfg.NAT.AutoNAT.EnableServer)
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("MinimalConfig", func(t *testing.T) {
		cfg := NewMinimalConfig()
		require.NotNil(t, cfg)
		assert.False(t, cfg.Discovery.EnableDHT)
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Log("✅ PresetConfigs 测试通过")
}

// TestApplyPreset 测试应用预设
func TestApplyPreset(t *testing.T) {
	cfg := NewConfig()

	err := ApplyPreset(cfg, "server")
	require.NoError(t, err)

	assert.True(t, cfg.Relay.EnableServer)
	assert.True(t, cfg.NAT.AutoNAT.EnableServer)

	t.Log("✅ ApplyPreset 测试通过")
}

// TestApplyPreset_Invalid 测试应用无效预设
func TestApplyPreset_Invalid(t *testing.T) {
	cfg := NewConfig()

	err := ApplyPreset(cfg, "invalid")
	assert.Error(t, err)

	t.Log("✅ ApplyPreset_Invalid 测试通过")
}

// TestValidateAll 测试完整配置验证
func TestValidateAll(t *testing.T) {
	cfg := NewConfig()
	err := ValidateAll(cfg)
	assert.NoError(t, err)

	t.Log("✅ ValidateAll 测试通过")
}

// TestValidateAndFix 测试配置自动修复
func TestValidateAndFix(t *testing.T) {
	cfg := NewConfig()

	// 设置一些问题配置
	cfg.ConnMgr.LowWater = 500
	cfg.ConnMgr.HighWater = 100 // 错误：low > high

	// 自动修复
	fixedCfg, err := ValidateAndFix(cfg)
	require.NoError(t, err)

	// 应该被修复
	assert.LessOrEqual(t, fixedCfg.ConnMgr.LowWater, fixedCfg.ConnMgr.HighWater)

	t.Log("✅ ValidateAndFix 测试通过")
}

// TestValidateCompatibility 测试配置兼容性验证
func TestValidateCompatibility(t *testing.T) {
	cfg := NewServerConfig()
	err := ValidateCompatibility(cfg)
	assert.NoError(t, err)

	t.Log("✅ ValidateCompatibility 测试通过")
}

// TestValidateForEnvironment 测试环境配置验证
func TestValidateForEnvironment(t *testing.T) {
	t.Run("Mobile", func(t *testing.T) {
		cfg := NewMobileConfig()
		err := ValidateForEnvironment(cfg, "mobile")
		assert.NoError(t, err)
	})

	t.Run("Desktop", func(t *testing.T) {
		cfg := NewConfig()
		err := ValidateForEnvironment(cfg, "desktop")
		assert.NoError(t, err)
	})

	t.Run("Server", func(t *testing.T) {
		cfg := NewServerConfig()
		err := ValidateForEnvironment(cfg, "server")
		assert.NoError(t, err)
	})

	t.Log("✅ ValidateForEnvironment 测试通过")
}

// TestCloneConfig 测试配置克隆
func TestCloneConfig(t *testing.T) {
	original := NewConfig()
	original.ConnMgr.HighWater = 1000

	cloned := CloneConfig(original)
	require.NotNil(t, cloned)

	// 修改克隆不影响原始
	cloned.ConnMgr.HighWater = 2000

	assert.Equal(t, 1000, original.ConnMgr.HighWater)
	assert.Equal(t, 2000, cloned.ConnMgr.HighWater)

	t.Log("✅ CloneConfig 测试通过")
}

// TestConvertForComponent 测试组件配置转换
func TestConvertForComponent(t *testing.T) {
	cfg := NewConfig()

	transport, err := ConvertForComponent(cfg, "transport")
	require.NoError(t, err)
	assert.Equal(t, cfg.Transport, transport)

	security, err := ConvertForComponent(cfg, "security")
	require.NoError(t, err)
	assert.Equal(t, cfg.Security, security)

	_, err = ConvertForComponent(cfg, "invalid")
	assert.Error(t, err)

	t.Log("✅ ConvertForComponent 测试通过")
}

// TestDurations 测试时间配置
func TestDurations(t *testing.T) {
	cfg := NewConfig()

	// Transport - 带有 json 标签的字段使用 config.Duration，需要 .Duration() 转换
	assert.Equal(t, 30*time.Second, cfg.Transport.DialTimeout.Duration())
	assert.Equal(t, 30*time.Second, cfg.Transport.QUIC.MaxIdleTimeout.Duration())

	// Security - 带有 json 标签的字段使用 config.Duration
	assert.Equal(t, 60*time.Second, cfg.Security.NegotiateTimeout.Duration())

	// NAT - 没有 json 标签的字段保持 time.Duration
	assert.Equal(t, 15*time.Second, cfg.NAT.AutoNAT.ProbeInterval)

	// ConnMgr - 带有 json 标签的字段使用 config.Duration
	assert.Equal(t, 20*time.Second, cfg.ConnMgr.GracePeriod.Duration())

	// Messaging - 没有 json 标签的字段保持 time.Duration
	assert.Equal(t, 30*time.Second, cfg.Messaging.Liveness.HeartbeatInterval)

	t.Log("✅ Durations 测试通过")
}
