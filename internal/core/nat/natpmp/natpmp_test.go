package natpmp

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
)

// ============================================================================
//                              Mapper 测试
// ============================================================================

func TestNewMapper(t *testing.T) {
	t.Run("使用默认配置", func(t *testing.T) {
		mapper := NewMapper()
		require.NotNil(t, mapper)
		assert.NotNil(t, mapper.mappings)
		assert.Greater(t, mapper.timeout, time.Duration(0))
		assert.Greater(t, mapper.refreshPeriod, time.Duration(0))
	})

	t.Run("使用默认配置", func(t *testing.T) {
		mapper := NewMapper()
		require.NotNil(t, mapper)
	})
}

func TestMapper_Name(t *testing.T) {
	mapper := NewMapper()
	assert.Equal(t, "nat-pmp", mapper.Name())
}

// ============================================================================
//                              映射管理测试
// ============================================================================

func TestMapper_MappingsStorage(t *testing.T) {
	mapper := NewMapper()

	// 初始应该为空
	mapper.mappingsMu.RLock()
	count := len(mapper.mappings)
	mapper.mappingsMu.RUnlock()
	assert.Equal(t, 0, count)

	// 添加一个映射（内部状态）
	mapper.mappingsMu.Lock()
	mapper.mappings["tcp:1234"] = &natif.Mapping{
		Protocol:     "tcp",
		InternalPort: 1234,
		ExternalPort: 5678,
		ExternalAddr: "1.2.3.4",
	}
	mapper.mappingsMu.Unlock()

	// 现在应该有一个映射
	mapper.mappingsMu.RLock()
	count = len(mapper.mappings)
	m := mapper.mappings["tcp:1234"]
	mapper.mappingsMu.RUnlock()

	assert.Equal(t, 1, count)
	assert.Equal(t, 1234, m.InternalPort)
}

// ============================================================================
//                              网关发现测试
// ============================================================================

func TestMapper_Available_NoGateway(t *testing.T) {
	mapper := NewMapper()
	mapper.timeout = 100 * time.Millisecond

	// 没有发现网关时通常返回 false
	// 但在有 NAT-PMP 网关的环境中可能返回 true
	_ = mapper.Available()
}

// ============================================================================
//                              端口映射测试
// ============================================================================

func TestMapper_AddMapping_NoGateway(t *testing.T) {
	mapper := NewMapper()
	mapper.timeout = 100 * time.Millisecond

	// 在没有 NAT-PMP 网关的环境中会返回错误
	// 在有 NAT-PMP 网关的环境中可能成功
	_, err := mapper.AddMapping("tcp", 1234, "test", time.Hour)
	// 我们只验证调用不会 panic
	_ = err
}

func TestMapper_DeleteMapping_NoGateway(t *testing.T) {
	mapper := NewMapper()
	mapper.timeout = 100 * time.Millisecond

	// 没有网关时删除应该静默成功（幂等）
	err := mapper.DeleteMapping("tcp", 1234)
	assert.NoError(t, err)
}

func TestMapper_DeleteMapping_NotExists(t *testing.T) {
	mapper := NewMapper()

	// 删除不存在的映射应该静默成功
	err := mapper.DeleteMapping("tcp", 9999)
	assert.NoError(t, err)
}

// ============================================================================
//                              外部地址测试
// ============================================================================

func TestMapper_GetExternalAddress(t *testing.T) {
	mapper := NewMapper()
	mapper.timeout = 100 * time.Millisecond

	// 在没有 NAT-PMP 网关的环境中可能返回错误
	// 在有 NAT-PMP 网关的环境中可能成功
	addr, err := mapper.GetExternalAddress()
	// 我们只验证调用不会 panic
	_ = err
	_ = addr
}

func TestMapper_GetExternalAddress_Cached(t *testing.T) {
	mapper := NewMapper()

	// 设置缓存
	mapper.clientMu.Lock()
	mapper.externalIP = "1.2.3.4"
	mapper.clientMu.Unlock()

	addr, err := mapper.GetExternalAddress()
	assert.NoError(t, err)
	assert.NotNil(t, addr)
	assert.Contains(t, addr.String(), "1.2.3.4")
}

// ============================================================================
//                              Stop 测试
// ============================================================================

func TestMapper_Close(t *testing.T) {
	mapper := NewMapper()

	// 添加一些映射
	mapper.mappingsMu.Lock()
	mapper.mappings["tcp:1234"] = &natif.Mapping{
		Protocol:     "tcp",
		InternalPort: 1234,
	}
	mapper.mappingsMu.Unlock()

	// Close 应该清理所有映射
	err := mapper.Close()
	assert.NoError(t, err)
	assert.True(t, mapper.closed)

	// 映射应该被清空
	mapper.mappingsMu.RLock()
	count := len(mapper.mappings)
	mapper.mappingsMu.RUnlock()
	assert.Equal(t, 0, count)
}

func TestMapper_Close_MultipleTimes(t *testing.T) {
	mapper := NewMapper()

	// 多次调用 Close 应该是安全的
	for i := 0; i < 3; i++ {
		err := mapper.Close()
		assert.NoError(t, err)
	}
}

// ============================================================================
//                              并发安全测试
// ============================================================================

func TestMapper_Concurrency(t *testing.T) {
	mapper := NewMapper()
	var wg sync.WaitGroup

	// 并发读写测试
	for i := 0; i < 10; i++ {
		wg.Add(2)

		go func(id int) {
			defer wg.Done()
			mapper.mappingsMu.RLock()
			_ = len(mapper.mappings)
			mapper.mappingsMu.RUnlock()
			_ = mapper.Name()
		}(i)

		go func(id int) {
			defer wg.Done()
			mapper.mappingsMu.Lock()
			mapper.mappings["tcp:"+string(rune('0'+id))] = &natif.Mapping{
				InternalPort: id,
			}
			mapper.mappingsMu.Unlock()
		}(i)
	}

	wg.Wait()
}

// ============================================================================
//                              本地 IP 获取测试
// ============================================================================

func TestGetLocalIP(t *testing.T) {
	ip := getLocalIP()
	if ip == "" {
		t.Skip("无法获取本地 IP")
	}

	// 验证是有效的 IP 地址
	parsed := net.ParseIP(ip)
	assert.NotNil(t, parsed)
}

// ============================================================================
//                              网关 IP 获取测试
// ============================================================================

func TestGetDefaultGateway(t *testing.T) {
	gateway, err := getDefaultGateway()
	// 可能成功也可能失败，取决于网络环境
	if err == nil && gateway != nil {
		t.Logf("默认网关: %s", gateway.String())
	}
}

// ============================================================================
//                              错误定义测试
// ============================================================================

func TestErrors(t *testing.T) {
	assert.NotNil(t, ErrNoGateway)
	assert.NotNil(t, ErrMappingFailed)
	assert.NotNil(t, ErrNATPMPNotSupported)
	assert.NotNil(t, ErrGatewayNotReady)
}

// ============================================================================
//                              ipAddr 测试
// ============================================================================

func TestIPAddr(t *testing.T) {
	t.Run("IPv4 地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("1.2.3.4"), 5678)

		assert.NotEmpty(t, addr.Network()) // Network() 返回协议类型
		assert.Contains(t, addr.String(), "1.2.3.4")
		assert.NotEmpty(t, addr.Bytes())
	})

	t.Run("公网地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("8.8.8.8"), 1234)

		assert.True(t, addr.IsPublic())
		assert.False(t, addr.IsPrivate())
		assert.False(t, addr.IsLoopback())
	})

	t.Run("私有地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("192.168.1.1"), 1234)

		assert.False(t, addr.IsPublic())
		assert.True(t, addr.IsPrivate())
		assert.False(t, addr.IsLoopback())
	})

	t.Run("回环地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("127.0.0.1"), 1234)

		assert.False(t, addr.IsPublic())
		assert.False(t, addr.IsPrivate())
		assert.True(t, addr.IsLoopback())
	})

	t.Run("Equal 比较", func(t *testing.T) {
		addr1 := newIPAddr(net.ParseIP("1.2.3.4"), 5678)
		addr2 := newIPAddr(net.ParseIP("1.2.3.4"), 5678)
		addr3 := newIPAddr(net.ParseIP("5.6.7.8"), 5678)

		assert.True(t, addr1.Equal(addr2))
		assert.False(t, addr1.Equal(addr3))
		assert.False(t, addr1.Equal(nil))
	})
}

// ============================================================================
//                              集成测试（需要 NAT-PMP 网关）
// ============================================================================

func TestMapper_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	mapper := NewMapper()

	// 检查 NAT-PMP 是否可用
	if !mapper.Available() {
		t.Skip("NAT-PMP 不可用")
	}

	t.Run("获取外部地址", func(t *testing.T) {
		addr, err := mapper.GetExternalAddress()
		if err != nil {
			t.Skipf("无法获取外部地址: %v", err)
		}
		assert.NotNil(t, addr)
		t.Logf("外部地址: %s", addr.String())
	})

	t.Run("添加和删除映射", func(t *testing.T) {
		externalPort, err := mapper.AddMapping("tcp", 12346, "test", time.Hour)
		if err != nil {
			t.Skipf("无法添加映射: %v", err)
		}
		t.Logf("映射: 12346 -> %d", externalPort)

		// 删除映射
		err = mapper.DeleteMapping("tcp", 12346)
		assert.NoError(t, err)
	})
}

// ============================================================================
//                              配置并发安全测试
// ============================================================================

func TestMapper_SetTimeout_Concurrent(t *testing.T) {
	mapper := NewMapper()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mapper.SetTimeout(time.Duration(i) * time.Second)
			_ = mapper.getTimeout()
		}(i)
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

func TestMapper_SetRefreshPeriod_Concurrent(t *testing.T) {
	mapper := NewMapper()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mapper.SetRefreshPeriod(time.Duration(i) * time.Minute)
			_ = mapper.getRefreshPeriod()
		}(i)
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

func TestMapper_Close_Concurrent(t *testing.T) {
	mapper := NewMapper()

	// 添加一些映射
	mapper.mappingsMu.Lock()
	mapper.mappings["tcp:1234"] = &natif.Mapping{
		Protocol:     "tcp",
		InternalPort: 1234,
	}
	mapper.mappingsMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mapper.Close()
		}()
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

// ============================================================================
//                              getLocalIP 测试
// ============================================================================

func TestGetLocalIP_Valid(t *testing.T) {
	ip := getLocalIP()
	assert.NotEmpty(t, ip)

	// 应该是有效的 IP 地址
	parsed := net.ParseIP(ip)
	assert.NotNil(t, parsed)
}

// ============================================================================
//                              ipAddr 扩展测试
// ============================================================================

func TestIPAddr_ZeroPort(t *testing.T) {
	addr := newIPAddr(net.ParseIP("1.2.3.4"), 0)

	// 端口为 0 时应该只返回 IP
	assert.Equal(t, "1.2.3.4", addr.String())
}

func TestIPAddr_IPv6WithPort(t *testing.T) {
	addr := newIPAddr(net.ParseIP("::1"), 8080)

	// IPv6 地址应该用方括号包围
	assert.Contains(t, addr.String(), "[")
	assert.Contains(t, addr.String(), "]")
	assert.Contains(t, addr.String(), "8080")
}
