package introspect

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	server := New(Config{})
	assert.NotNil(t, server)
	assert.Equal(t, DefaultAddr, server.config.Addr)

	server = New(Config{Addr: "127.0.0.1:8080"})
	assert.Equal(t, "127.0.0.1:8080", server.config.Addr)
}

func TestServer_StartStop(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:0"}) // 使用随机端口

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	assert.True(t, server.running)

	// 获取实际地址
	addr := server.Addr()
	assert.NotEmpty(t, addr)
	assert.NotEqual(t, "127.0.0.1:0", addr)

	// 重复启动应该无效
	err = server.Start(ctx)
	require.NoError(t, err)

	// 停止
	err = server.Stop()
	require.NoError(t, err)
	assert.False(t, server.running)

	// 重复停止应该无效
	err = server.Stop()
	require.NoError(t, err)
}

func TestServer_HealthEndpoint(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:0"})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 请求健康检查
	resp, err := http.Get("http://" + server.Addr() + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)

	assert.Equal(t, "degraded", health.Status) // 没有 Host，所以是 degraded
	assert.NotEmpty(t, health.Uptime)
}

func TestServer_IntrospectEndpoint(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:0"})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 请求完整诊断
	resp, err := http.Get("http://" + server.Addr() + "/debug/introspect")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var introspect IntrospectResponse
	err = json.NewDecoder(resp.Body).Decode(&introspect)
	require.NoError(t, err)

	assert.NotEmpty(t, introspect.Uptime)
	assert.NotNil(t, introspect.Runtime)
}

func TestServer_RuntimeEndpoint(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:0"})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 请求运行时信息
	resp, err := http.Get("http://" + server.Addr() + "/debug/introspect/runtime")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var runtime RuntimeInfo
	err = json.NewDecoder(resp.Body).Decode(&runtime)
	require.NoError(t, err)

	assert.NotEmpty(t, runtime.GoVersion)
	assert.Greater(t, runtime.NumGoroutine, 0)
	assert.Greater(t, runtime.NumCPU, 0)
	assert.Greater(t, runtime.MemAlloc, uint64(0))
}

func TestServer_BandwidthEndpoint(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:0"})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 请求带宽信息（没有 BandwidthReporter，应该返回空数据）
	resp, err := http.Get("http://" + server.Addr() + "/debug/introspect/bandwidth")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var bandwidth BandwidthInfo
	err = json.NewDecoder(resp.Body).Decode(&bandwidth)
	require.NoError(t, err)
}

func TestServer_NodeEndpoint_NoHost(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:0"})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 请求节点信息（没有 Host）
	resp, err := http.Get("http://" + server.Addr() + "/debug/introspect/node")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServer_ConnectionsEndpoint_NoHost(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:0"})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 请求连接信息（没有 Host）
	resp, err := http.Get("http://" + server.Addr() + "/debug/introspect/connections")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServer_MethodNotAllowed(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:0"})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 使用 POST 方法（应该被拒绝）
	resp, err := http.Post("http://"+server.Addr()+"/health", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestServer_CustomHandlers(t *testing.T) {
	customCalled := false
	server := New(Config{
		Addr: "127.0.0.1:0",
		CustomHandlers: map[string]http.HandlerFunc{
			"/custom": func(w http.ResponseWriter, r *http.Request) {
				customCalled = true
				w.Write([]byte("custom response"))
			},
		},
	})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 请求自定义端点
	resp, err := http.Get("http://" + server.Addr() + "/custom")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, customCalled)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "custom response", string(body))
}

func TestServer_PprofEndpoint(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:0"})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 请求 pprof 索引
	resp, err := http.Get("http://" + server.Addr() + "/debug/pprof/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_Addr(t *testing.T) {
	server := New(Config{Addr: "127.0.0.1:8888"})

	// 未启动时返回配置地址
	addr := server.Addr()
	assert.Equal(t, "127.0.0.1:8888", addr)

	// 使用随机端口
	server = New(Config{Addr: "127.0.0.1:0"})
	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 启动后返回实际地址
	addr = server.Addr()
	assert.NotEqual(t, "127.0.0.1:0", addr)
	assert.Contains(t, addr, "127.0.0.1:")
}

func TestCollectRuntimeInfo(t *testing.T) {
	server := New(Config{})
	info := server.collectRuntimeInfo()

	assert.NotNil(t, info)
	assert.NotEmpty(t, info.GoVersion)
	assert.Greater(t, info.NumGoroutine, 0)
	assert.Greater(t, info.NumCPU, 0)
}

// mockBandwidthReporter 模拟带宽报告器
type mockBandwidthReporter struct {
	totalIn  int64
	totalOut int64
}

func (m *mockBandwidthReporter) GetBandwidthForPeer(peer string) (in, out int64) {
	return 100, 200
}

func (m *mockBandwidthReporter) GetBandwidthTotals() (in, out int64) {
	return m.totalIn, m.totalOut
}

func TestServer_BandwidthEndpoint_WithReporter(t *testing.T) {
	reporter := &mockBandwidthReporter{
		totalIn:  1000,
		totalOut: 2000,
	}

	server := New(Config{
		Addr:              "127.0.0.1:0",
		BandwidthReporter: reporter,
	})

	ctx := context.Background()
	err := server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 等待服务启动
	time.Sleep(50 * time.Millisecond)

	// 请求带宽信息
	resp, err := http.Get("http://" + server.Addr() + "/debug/introspect/bandwidth")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var bandwidth BandwidthInfo
	err = json.NewDecoder(resp.Body).Decode(&bandwidth)
	require.NoError(t, err)

	assert.Equal(t, int64(1000), bandwidth.TotalIn)
	assert.Equal(t, int64(2000), bandwidth.TotalOut)
}
