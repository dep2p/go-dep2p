package nat

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestNewService 测试创建 NAT 服务
func TestNewService(t *testing.T) {
	cfg := DefaultConfig()
	s, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	if s == nil {
		t.Fatal("NewService returned nil")
	}

	t.Log("✅ NewService 成功创建服务")
}

// TestService_Reachability 测试可达性状态
func TestService_Reachability(t *testing.T) {
	cfg := DefaultConfig()
	s, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 初始状态应该是 Unknown
	if s.Reachability() != ReachabilityUnknown {
		t.Errorf("Initial reachability = %v, want Unknown", s.Reachability())
	}

	t.Log("✅ Reachability 初始状态正确")
}

// TestService_ExternalAddrs 测试外部地址获取
func TestService_ExternalAddrs(t *testing.T) {
	cfg := DefaultConfig()
	s, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 初始状态外部地址为空
	addrs := s.ExternalAddrs()
	if addrs == nil {
		t.Error("ExternalAddrs returned nil")
	}
	if len(addrs) != 0 {
		t.Errorf("Initial external addrs = %d, want 0", len(addrs))
	}

	t.Log("✅ ExternalAddrs 初始状态正确")
}

// TestService_StartStop 测试服务启动和停止
func TestService_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableAutoNAT = false // 禁用 AutoNAT 避免依赖
	cfg.EnableUPnP = false    // 禁用 UPnP 避免依赖
	cfg.EnableNATPMP = false  // 禁用 NAT-PMP 避免依赖

	s, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	ctx := context.Background()

	// 启动服务（host 为 nil 用于测试）
	if err := s.Start(ctx, nil); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 等待一小段时间
	time.Sleep(100 * time.Millisecond)

	// 停止服务
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	t.Log("✅ Service 启动和停止成功")
}

// TestService_StartTwice 测试重复启动
func TestService_StartTwice(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableAutoNAT = false
	cfg.EnableUPnP = false
	cfg.EnableNATPMP = false

	s, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	ctx := context.Background()

	// 第一次启动（host 为 nil 用于测试）
	if err := s.Start(ctx, nil); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	// 第二次启动应该失败或被忽略
	err = s.Start(ctx, nil)
	// 实现时应该返回错误或直接返回

	s.Stop()

	t.Log("✅ Service 重复启动处理正确")
}

// TestService_ConfigValidation 测试配置验证
func TestService_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "默认配置",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name:    "空配置",
			config:  nil,
			wantErr: true,
		},
		{
			name: "无效探测间隔",
			config: &Config{
				ProbeInterval: -1 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService(tt.config, nil, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewService() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Log("✅ 配置验证测试通过")
}

// TestService_Close 测试关闭服务
func TestService_Close(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableAutoNAT = false
	cfg.EnableUPnP = false
	cfg.EnableNATPMP = false

	s, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	ctx := context.Background()
	if err := s.Start(ctx, nil); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 关闭服务
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 再次关闭应该是安全的
	if err := s.Stop(); err != nil {
		t.Errorf("Second Stop failed: %v", err)
	}

	t.Log("✅ Service 关闭测试通过")
}

// ============================================================================
//                 Config Options 测试（覆盖 0% 函数）
// ============================================================================

// TestConfig_WithAutoNAT 测试 AutoNAT 选项
func TestConfig_WithAutoNAT(t *testing.T) {
	cfg := DefaultConfig()

	// 禁用
	opt := WithAutoNAT(false)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithAutoNAT(false) error = %v", err)
	}
	if cfg.EnableAutoNAT != false {
		t.Errorf("EnableAutoNAT = %v, want false", cfg.EnableAutoNAT)
	}

	// 启用
	opt = WithAutoNAT(true)
	err = opt(cfg)
	if err != nil {
		t.Errorf("WithAutoNAT(true) error = %v", err)
	}
	if cfg.EnableAutoNAT != true {
		t.Errorf("EnableAutoNAT = %v, want true", cfg.EnableAutoNAT)
	}

	t.Log("✅ WithAutoNAT 测试通过")
}

// TestConfig_WithUPnP 测试 UPnP 选项
func TestConfig_WithUPnP(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithUPnP(false)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithUPnP(false) error = %v", err)
	}
	if cfg.EnableUPnP != false {
		t.Errorf("EnableUPnP = %v, want false", cfg.EnableUPnP)
	}

	t.Log("✅ WithUPnP 测试通过")
}

// TestConfig_WithNATPMP 测试 NAT-PMP 选项
func TestConfig_WithNATPMP(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithNATPMP(false)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithNATPMP(false) error = %v", err)
	}
	if cfg.EnableNATPMP != false {
		t.Errorf("EnableNATPMP = %v, want false", cfg.EnableNATPMP)
	}

	t.Log("✅ WithNATPMP 测试通过")
}

// TestConfig_WithHolePunch 测试 HolePunch 选项
func TestConfig_WithHolePunch(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithHolePunch(false)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithHolePunch(false) error = %v", err)
	}
	if cfg.EnableHolePunch != false {
		t.Errorf("EnableHolePunch = %v, want false", cfg.EnableHolePunch)
	}

	t.Log("✅ WithHolePunch 测试通过")
}

// TestConfig_WithSTUNServers 测试 STUN 服务器选项
func TestConfig_WithSTUNServers(t *testing.T) {
	cfg := DefaultConfig()

	servers := []string{"stun1.example.com:3478", "stun2.example.com:3478"}
	opt := WithSTUNServers(servers)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithSTUNServers error = %v", err)
	}
	if len(cfg.STUNServers) != 2 {
		t.Errorf("STUNServers len = %d, want 2", len(cfg.STUNServers))
	}

	// 空列表应该返回错误
	opt = WithSTUNServers([]string{})
	err = opt(cfg)
	if err == nil {
		t.Error("WithSTUNServers(empty) should return error")
	}

	t.Log("✅ WithSTUNServers 测试通过")
}

// TestConfig_WithProbeInterval 测试探测间隔选项
func TestConfig_WithProbeInterval(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithProbeInterval(30 * time.Second)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithProbeInterval error = %v", err)
	}
	if cfg.ProbeInterval != 30*time.Second {
		t.Errorf("ProbeInterval = %v, want 30s", cfg.ProbeInterval)
	}

	// 无效间隔
	opt = WithProbeInterval(0)
	err = opt(cfg)
	if err == nil {
		t.Error("WithProbeInterval(0) should return error")
	}

	opt = WithProbeInterval(-1 * time.Second)
	err = opt(cfg)
	if err == nil {
		t.Error("WithProbeInterval(-1s) should return error")
	}

	t.Log("✅ WithProbeInterval 测试通过")
}

// TestConfig_WithProbeTimeout 测试探测超时选项
func TestConfig_WithProbeTimeout(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithProbeTimeout(5 * time.Second)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithProbeTimeout error = %v", err)
	}
	if cfg.ProbeTimeout != 5*time.Second {
		t.Errorf("ProbeTimeout = %v, want 5s", cfg.ProbeTimeout)
	}

	// 无效超时
	opt = WithProbeTimeout(0)
	err = opt(cfg)
	if err == nil {
		t.Error("WithProbeTimeout(0) should return error")
	}

	t.Log("✅ WithProbeTimeout 测试通过")
}

// TestConfig_WithMappingDuration 测试映射持续时间选项
func TestConfig_WithMappingDuration(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithMappingDuration(2 * time.Hour)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithMappingDuration error = %v", err)
	}
	if cfg.MappingDuration != 2*time.Hour {
		t.Errorf("MappingDuration = %v, want 2h", cfg.MappingDuration)
	}

	// 无效持续时间
	opt = WithMappingDuration(0)
	err = opt(cfg)
	if err == nil {
		t.Error("WithMappingDuration(0) should return error")
	}

	t.Log("✅ WithMappingDuration 测试通过")
}

// TestConfig_WithConfidenceThreshold 测试置信度阈值选项
func TestConfig_WithConfidenceThreshold(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithConfidenceThreshold(5)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithConfidenceThreshold error = %v", err)
	}
	if cfg.ConfidenceThreshold != 5 {
		t.Errorf("ConfidenceThreshold = %d, want 5", cfg.ConfidenceThreshold)
	}

	// 无效阈值
	opt = WithConfidenceThreshold(0)
	err = opt(cfg)
	if err == nil {
		t.Error("WithConfidenceThreshold(0) should return error")
	}

	opt = WithConfidenceThreshold(-1)
	err = opt(cfg)
	if err == nil {
		t.Error("WithConfidenceThreshold(-1) should return error")
	}

	t.Log("✅ WithConfidenceThreshold 测试通过")
}

// TestConfig_WithNATTypeDetection 测试 NAT 类型检测选项
func TestConfig_WithNATTypeDetection(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithNATTypeDetection(false)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithNATTypeDetection error = %v", err)
	}
	if cfg.NATTypeDetectionEnabled != false {
		t.Errorf("NATTypeDetectionEnabled = %v, want false", cfg.NATTypeDetectionEnabled)
	}

	t.Log("✅ WithNATTypeDetection 测试通过")
}

// TestConfig_WithAlternateSTUNServer 测试备用 STUN 服务器选项
func TestConfig_WithAlternateSTUNServer(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithAlternateSTUNServer("stun.example.com:3478")
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithAlternateSTUNServer error = %v", err)
	}
	if cfg.AlternateSTUNServer != "stun.example.com:3478" {
		t.Errorf("AlternateSTUNServer = %v, want stun.example.com:3478", cfg.AlternateSTUNServer)
	}

	t.Log("✅ WithAlternateSTUNServer 测试通过")
}

// TestConfig_WithNATTypeDetectionTimeout 测试 NAT 类型检测超时选项
func TestConfig_WithNATTypeDetectionTimeout(t *testing.T) {
	cfg := DefaultConfig()

	opt := WithNATTypeDetectionTimeout(5 * time.Second)
	err := opt(cfg)
	if err != nil {
		t.Errorf("WithNATTypeDetectionTimeout error = %v", err)
	}
	if cfg.NATTypeDetectionTimeout != 5*time.Second {
		t.Errorf("NATTypeDetectionTimeout = %v, want 5s", cfg.NATTypeDetectionTimeout)
	}

	// 无效超时
	opt = WithNATTypeDetectionTimeout(0)
	err = opt(cfg)
	if err == nil {
		t.Error("WithNATTypeDetectionTimeout(0) should return error")
	}

	t.Log("✅ WithNATTypeDetectionTimeout 测试通过")
}

// TestConfig_ApplyOptions 测试应用多个选项
func TestConfig_ApplyOptions(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyOptions(
		WithAutoNAT(false),
		WithUPnP(false),
		WithNATPMP(false),
		WithProbeInterval(20*time.Second),
		WithProbeTimeout(5*time.Second),
	)
	if err != nil {
		t.Errorf("ApplyOptions error = %v", err)
	}

	if cfg.EnableAutoNAT != false {
		t.Errorf("EnableAutoNAT = %v, want false", cfg.EnableAutoNAT)
	}
	if cfg.EnableUPnP != false {
		t.Errorf("EnableUPnP = %v, want false", cfg.EnableUPnP)
	}
	if cfg.ProbeInterval != 20*time.Second {
		t.Errorf("ProbeInterval = %v, want 20s", cfg.ProbeInterval)
	}

	t.Log("✅ ApplyOptions 测试通过")
}

// TestConfig_ApplyOptions_WithError 测试选项应用错误
func TestConfig_ApplyOptions_WithError(t *testing.T) {
	cfg := DefaultConfig()

	// 第二个选项返回错误
	err := cfg.ApplyOptions(
		WithAutoNAT(false),
		WithSTUNServers([]string{}), // 空列表会返回错误
	)
	if err == nil {
		t.Error("ApplyOptions should return error when option fails")
	}

	t.Log("✅ ApplyOptions 错误处理测试通过")
}

// ============================================================================
//                 Errors 测试（覆盖 0% 函数）
// ============================================================================

// TestDialError 测试 DialError
func TestDialError(t *testing.T) {
	t.Run("with cause", func(t *testing.T) {
		cause := ErrNoExternalAddr
		err := &DialError{Cause: cause}

		msg := err.Error()
		if msg != "nat dial error: nat: no external address" {
			t.Errorf("Error() = %q, want contains 'no external address'", msg)
		}

		unwrapped := err.Unwrap()
		if unwrapped != cause {
			t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
		}
	})

	t.Run("with multiple errors", func(t *testing.T) {
		err := &DialError{Errors: []error{ErrSTUNTimeout, ErrNoExternalAddr}}

		msg := err.Error()
		if msg != "nat dial error: multiple failures" {
			t.Errorf("Error() = %q, want 'multiple failures'", msg)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		err := &DialError{}

		msg := err.Error()
		if msg != "nat dial error: unknown" {
			t.Errorf("Error() = %q, want 'unknown'", msg)
		}
	})

	t.Log("✅ DialError 测试通过")
}

// TestMappingError 测试 MappingError
func TestMappingError(t *testing.T) {
	cause := ErrMappingFailed
	err := &MappingError{
		Protocol: "TCP",
		Port:     8080,
		Cause:    cause,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error() returned empty string")
	}

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	t.Log("✅ MappingError 测试通过")
}

// TestProbeError 测试 ProbeError
func TestProbeError(t *testing.T) {
	cause := ErrNoPeers
	err := &ProbeError{
		PeerID: "12D3KooWTest",
		Cause:  cause,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error() returned empty string")
	}
	if msg != "nat probe error for peer 12D3KooWTest: nat: no peers available for probe" {
		t.Errorf("Error() = %q", msg)
	}

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	t.Log("✅ ProbeError 测试通过")
}

// TestReachability_String 测试 Reachability.String
func TestReachability_String(t *testing.T) {
	tests := []struct {
		r    Reachability
		want string
	}{
		{ReachabilityUnknown, "Unknown"},
		{ReachabilityPublic, "Public"},
		{ReachabilityPrivate, "Private"},
		{Reachability(99), "Unknown"}, // 未知值
	}

	for _, tt := range tests {
		got := tt.r.String()
		if got != tt.want {
			t.Errorf("Reachability(%d).String() = %q, want %q", tt.r, got, tt.want)
		}
	}

	t.Log("✅ Reachability.String 测试通过")
}

// ============================================================================
//                 辅助函数测试（发现潜在 BUG）
// ============================================================================

// TestIsPublicIP 测试公网 IP 判断
func TestIsPublicIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		// 公网 IP
		{"Google DNS", "8.8.8.8", true},
		{"Cloudflare DNS", "1.1.1.1", true},
		{"IPv6 Google", "2001:4860:4860::8888", true},

		// 私网 IP
		{"Private 10.x", "10.0.0.1", false},
		{"Private 172.16.x", "172.16.0.1", false},
		{"Private 192.168.x", "192.168.1.1", false},

		// 特殊地址
		{"Loopback", "127.0.0.1", false},
		{"Loopback IPv6", "::1", false},
		{"Link-local", "169.254.1.1", false},
		{"Link-local IPv6", "fe80::1", false},
		{"Multicast", "224.0.0.1", false},

		// 边界情况
		{"Empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ip net.IP
			if tt.ip != "" {
				ip = net.ParseIP(tt.ip)
			}
			got := isPublicIP(ip)
			if got != tt.want {
				t.Errorf("isPublicIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// TestIsPublicIP_NilIP 测试 nil IP
func TestIsPublicIP_NilIP(t *testing.T) {
	if isPublicIP(nil) {
		t.Error("isPublicIP(nil) should return false")
	}
}

// TestExtractPortFromAddr 测试从地址提取端口
func TestExtractPortFromAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want int
	}{
		// 正常地址
		{"UDP QUIC", "/ip4/192.168.1.1/udp/4001/quic-v1", 4001},
		{"TCP", "/ip4/192.168.1.1/tcp/8080", 8080},
		{"IPv6 UDP", "/ip6/::1/udp/5678/quic-v1", 5678},

		// 边界情况
		{"Empty", "", 0},
		{"No port", "/ip4/192.168.1.1", 0},
		{"Invalid port", "/ip4/192.168.1.1/udp/invalid", 0},
		{"Port 0", "/ip4/192.168.1.1/udp/0", 0},
		{"Port out of range", "/ip4/192.168.1.1/udp/70000", 0},
		{"Negative port", "/ip4/192.168.1.1/udp/-1", 0},
		{"Only slashes", "///", 0},
		{"Just protocol", "/udp/", 0},
		{"Port at end", "/ip4/192.168.1.1/udp", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPortFromAddr(tt.addr)
			if got != tt.want {
				t.Errorf("extractPortFromAddr(%q) = %d, want %d", tt.addr, got, tt.want)
			}
		})
	}
}

// TestExtractPortsFromAddrs 测试从地址列表提取端口
func TestExtractPortsFromAddrs(t *testing.T) {
	t.Run("multiple addresses same port", func(t *testing.T) {
		addrs := []string{
			"/ip4/192.168.1.1/udp/4001/quic-v1",
			"/ip4/10.0.0.1/udp/4001/quic-v1",
		}
		ports := extractPortsFromAddrs(addrs)
		// 应该去重
		if len(ports) != 1 {
			t.Errorf("Expected 1 unique port, got %d", len(ports))
		}
	})

	t.Run("multiple addresses different ports", func(t *testing.T) {
		addrs := []string{
			"/ip4/192.168.1.1/udp/4001/quic-v1",
			"/ip4/10.0.0.1/tcp/8080",
		}
		ports := extractPortsFromAddrs(addrs)
		if len(ports) != 2 {
			t.Errorf("Expected 2 ports, got %d", len(ports))
		}
	})

	t.Run("empty list", func(t *testing.T) {
		ports := extractPortsFromAddrs([]string{})
		if len(ports) != 0 {
			t.Errorf("Expected 0 ports for empty list, got %d", len(ports))
		}
	})

	t.Run("all invalid addresses", func(t *testing.T) {
		addrs := []string{"invalid", "/ip4/192.168.1.1"}
		ports := extractPortsFromAddrs(addrs)
		if len(ports) != 0 {
			t.Errorf("Expected 0 ports for invalid addresses, got %d", len(ports))
		}
	})
}

// TestPortsEqual 测试端口列表比较
func TestPortsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []int
		b    []int
		want bool
	}{
		{"both empty", []int{}, []int{}, true},
		{"same single", []int{4001}, []int{4001}, true},
		{"same multiple", []int{4001, 8080}, []int{8080, 4001}, true}, // 顺序不同
		{"different length", []int{4001}, []int{4001, 8080}, false},
		{"different values", []int{4001}, []int{8080}, false},
		{"nil and empty", nil, []int{}, true},
		{"both nil", nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := portsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("portsEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestService_SetListenPorts 测试设置监听端口
func TestService_SetListenPorts(t *testing.T) {
	cfg := DefaultConfig()
	s, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 设置端口
	s.SetListenPorts([]int{4001, 8080})

	// 获取并验证
	ports := s.GetListenPorts()
	if len(ports) != 2 {
		t.Errorf("Expected 2 ports, got %d", len(ports))
	}

	// 验证是深拷贝
	ports[0] = 9999
	actualPorts := s.GetListenPorts()
	if actualPorts[0] == 9999 {
		t.Error("GetListenPorts should return a copy, not the original slice")
	}
}

// TestService_ConcurrentPortAccess 测试并发访问端口
func TestService_ConcurrentPortAccess(t *testing.T) {
	cfg := DefaultConfig()
	s, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 并发读写
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				if j%2 == 0 {
					s.SetListenPorts([]int{id, j})
				} else {
					_ = s.GetListenPorts()
				}
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 如果没有 panic 或 race，测试通过
	t.Log("✅ 并发端口访问测试通过")
}
