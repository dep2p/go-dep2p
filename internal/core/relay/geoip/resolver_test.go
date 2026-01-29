package geoip

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, 1000, config.CacheSize)
	assert.True(t, config.Enabled)
}

func TestGeoInfo_ToRegionString(t *testing.T) {
	tests := []struct {
		name     string
		info     *GeoInfo
		expected string
	}{
		{
			name:     "continent code",
			info:     &GeoInfo{ContinentCode: "AS", CountryCode: "CN"},
			expected: "AS",
		},
		{
			name:     "country code only",
			info:     &GeoInfo{CountryCode: "US"},
			expected: "US",
		},
		{
			name:     "unknown",
			info:     &GeoInfo{},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.info.ToRegionString())
		})
	}
}

func TestStubResolver(t *testing.T) {
	resolver := NewStubResolver()

	// 设置映射
	resolver.SetMapping("1.2.3.4", &GeoInfo{
		ContinentCode: "AS",
		CountryCode:   "CN",
		CountryName:   "China",
	})

	// 查询
	info, err := resolver.LookupString("1.2.3.4")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "AS", info.ContinentCode)
	assert.Equal(t, "CN", info.CountryCode)

	// 查询不存在的 IP
	info, err = resolver.LookupString("5.6.7.8")
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestStubResolver_SetRegion(t *testing.T) {
	resolver := NewStubResolver()

	// 快捷设置
	resolver.SetRegion("1.2.3.4", "EU")

	info, err := resolver.LookupString("1.2.3.4")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "EU", info.ContinentCode)
}

func TestStubResolver_Disabled(t *testing.T) {
	resolver := NewStubResolver()
	resolver.SetRegion("1.2.3.4", "EU")
	resolver.SetEnabled(false)

	// 禁用后返回 nil
	info, err := resolver.LookupString("1.2.3.4")
	require.NoError(t, err)
	assert.Nil(t, info)

	assert.False(t, resolver.IsAvailable())
}

func TestStubResolver_Clear(t *testing.T) {
	resolver := NewStubResolver()
	resolver.SetRegion("1.2.3.4", "EU")

	info, _ := resolver.LookupString("1.2.3.4")
	assert.NotNil(t, info)

	resolver.Clear()

	info, _ = resolver.LookupString("1.2.3.4")
	assert.Nil(t, info)
}

func TestStubResolver_Lookup(t *testing.T) {
	resolver := NewStubResolver()
	resolver.SetRegion("192.168.1.1", "LOCAL")

	ip := net.ParseIP("192.168.1.1")
	info, err := resolver.Lookup(ip)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "LOCAL", info.ContinentCode)
}

func TestSimpleResolver(t *testing.T) {
	config := Config{
		CacheSize: 100,
		Enabled:   true,
	}
	resolver := NewSimpleResolver(config)

	// 私有地址
	info, err := resolver.LookupString("192.168.1.1")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "LOCAL", info.ContinentCode)

	// 环回地址
	info, err = resolver.LookupString("127.0.0.1")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "LOCAL", info.ContinentCode)
}

func TestSimpleResolver_Disabled(t *testing.T) {
	config := Config{
		CacheSize: 100,
		Enabled:   false,
	}
	resolver := NewSimpleResolver(config)

	info, err := resolver.LookupString("8.8.8.8")
	require.NoError(t, err)
	assert.Nil(t, info)

	assert.False(t, resolver.IsAvailable())
}

func TestSimpleResolver_Cache(t *testing.T) {
	config := Config{
		CacheSize: 100,
		Enabled:   true,
	}
	resolver := NewSimpleResolver(config)

	// 第一次查询
	info1, _ := resolver.LookupString("8.8.8.8")

	// 第二次查询（应该命中缓存）
	info2, _ := resolver.LookupString("8.8.8.8")

	assert.Equal(t, info1, info2)

	// 清空缓存
	resolver.ClearCache()
}

func TestSimpleResolver_InvalidIP(t *testing.T) {
	config := DefaultConfig()
	resolver := NewSimpleResolver(config)

	_, err := resolver.LookupString("invalid")
	assert.Error(t, err)
}

func TestRegionResolver(t *testing.T) {
	config := Config{
		CacheSize: 100,
		Enabled:   true,
	}
	resolver := NewRegionResolver(config)

	// 私有地址（默认映射）
	info, err := resolver.LookupString("192.168.1.1")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "LOCAL", info.ContinentCode)

	// Google DNS（默认映射）
	info, err = resolver.LookupString("8.8.8.8")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "NA", info.ContinentCode)
}

func TestRegionResolver_AddMapping(t *testing.T) {
	config := Config{
		CacheSize: 100,
		Enabled:   true,
	}
	resolver := NewRegionResolver(config)

	// 添加自定义映射
	err := resolver.AddMapping("203.0.113.0/24", &GeoInfo{
		ContinentCode: "AS",
		CountryCode:   "JP",
		CountryName:   "Japan",
	})
	require.NoError(t, err)

	// 查询
	info, err := resolver.LookupString("203.0.113.100")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "AS", info.ContinentCode)
	assert.Equal(t, "JP", info.CountryCode)
}

func TestRegionResolver_InvalidCIDR(t *testing.T) {
	config := DefaultConfig()
	resolver := NewRegionResolver(config)

	err := resolver.AddMapping("invalid", &GeoInfo{})
	assert.Error(t, err)
}

func TestRegionResolver_UnknownIP(t *testing.T) {
	config := Config{
		CacheSize: 100,
		Enabled:   true,
	}
	resolver := NewRegionResolver(config)

	// 查询未映射的 IP
	info, err := resolver.LookupString("198.51.100.1")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "UNKNOWN", info.ContinentCode)
}

func TestRegionResolver_Disabled(t *testing.T) {
	config := Config{
		CacheSize: 100,
		Enabled:   false,
	}
	resolver := NewRegionResolver(config)

	info, err := resolver.LookupString("8.8.8.8")
	require.NoError(t, err)
	assert.Nil(t, info)

	assert.False(t, resolver.IsAvailable())
}

func TestRegionResolver_Cache(t *testing.T) {
	config := Config{
		CacheSize: 100,
		Enabled:   true,
	}
	resolver := NewRegionResolver(config)

	// 第一次查询
	info1, _ := resolver.LookupString("8.8.8.8")

	// 第二次查询（应该命中缓存）
	info2, _ := resolver.LookupString("8.8.8.8")

	assert.Equal(t, info1, info2)

	// 清空缓存
	resolver.ClearCache()
}

func TestRegionResolver_Close(t *testing.T) {
	config := DefaultConfig()
	resolver := NewRegionResolver(config)

	err := resolver.Close()
	assert.NoError(t, err)
}
