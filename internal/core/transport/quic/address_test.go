package quic

import (
	"net"
	"testing"
)

// TestNewAddress 测试地址创建
func TestNewAddress(t *testing.T) {
	tests := []struct {
		name            string
		host            string
		port            int
		expectedNetwork string
		expectedString  string
	}{
		{
			name:            "IPv4 地址",
			host:            "192.168.1.1",
			port:            8080,
			expectedNetwork: "ip4",
			expectedString:  "192.168.1.1:8080",
		},
		{
			name:            "IPv6 地址",
			host:            "::1",
			port:            8080,
			expectedNetwork: "ip6",
			expectedString:  "[::1]:8080",
		},
		{
			name:            "localhost",
			host:            "127.0.0.1",
			port:            9000,
			expectedNetwork: "ip4",
			expectedString:  "127.0.0.1:9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := NewAddress(tt.host, tt.port)

			if addr.Network() != tt.expectedNetwork {
				t.Errorf("Network() = %s，期望 %s", addr.Network(), tt.expectedNetwork)
			}

			if addr.String() != tt.expectedString {
				t.Errorf("String() = %s，期望 %s", addr.String(), tt.expectedString)
			}

			if addr.Host() != tt.host {
				t.Errorf("Host() = %s，期望 %s", addr.Host(), tt.host)
			}

			if addr.Port() != tt.port {
				t.Errorf("Port() = %d，期望 %d", addr.Port(), tt.port)
			}
		})
	}
}

// TestParseAddress 测试地址解析
func TestParseAddress(t *testing.T) {
	tests := []struct {
		name        string
		addrStr     string
		expectError bool
		host        string
		port        int
	}{
		{
			name:        "有效的 IPv4 地址",
			addrStr:     "192.168.1.1:8080",
			expectError: false,
			host:        "192.168.1.1",
			port:        8080,
		},
		{
			name:        "有效的 IPv6 地址",
			addrStr:     "[::1]:8080",
			expectError: false,
			host:        "::1",
			port:        8080,
		},
		{
			name:        "无效的地址格式",
			addrStr:     "invalid",
			expectError: true,
		},
		{
			name:        "无效的端口",
			addrStr:     "127.0.0.1:invalid",
			expectError: true,
		},
		{
			name:        "端口超出范围",
			addrStr:     "127.0.0.1:70000",
			expectError: true,
		},
		{
			name:        "负端口",
			addrStr:     "127.0.0.1:-1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := ParseAddress(tt.addrStr)

			if tt.expectError {
				if err == nil {
					t.Error("期望返回错误")
				}
				return
			}

			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}

			if addr.Host() != tt.host {
				t.Errorf("Host() = %s，期望 %s", addr.Host(), tt.host)
			}

			if addr.Port() != tt.port {
				t.Errorf("Port() = %d，期望 %d", addr.Port(), tt.port)
			}
		})
	}
}

// TestMustParseAddress 测试 MustParseAddress
func TestMustParseAddress(t *testing.T) {
	// 测试有效地址
	addr := MustParseAddress("127.0.0.1:8080")
	if addr == nil {
		t.Fatal("地址不应为 nil")
	}

	// 测试无效地址（应 panic）
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseAddress 应对无效地址 panic")
		}
	}()
	MustParseAddress("invalid")
}

// TestFromNetAddr 测试从 net.Addr 创建地址
func TestFromNetAddr(t *testing.T) {
	// UDP 地址
	udpAddr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 8080,
	}
	addr, err := FromNetAddr(udpAddr)
	if err != nil {
		t.Fatalf("转换 UDP 地址失败: %v", err)
	}
	if addr.String() != "127.0.0.1:8080" {
		t.Errorf("String() = %s，期望 127.0.0.1:8080", addr.String())
	}

	// TCP 地址
	tcpAddr := &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.1"),
		Port: 9000,
	}
	addr, err = FromNetAddr(tcpAddr)
	if err != nil {
		t.Fatalf("转换 TCP 地址失败: %v", err)
	}
	if addr.String() != "192.168.1.1:9000" {
		t.Errorf("String() = %s，期望 192.168.1.1:9000", addr.String())
	}
}

// TestAddressEqual 测试地址相等性
func TestAddressEqual(t *testing.T) {
	addr1 := NewAddress("127.0.0.1", 8080)
	addr2 := NewAddress("127.0.0.1", 8080)
	addr3 := NewAddress("127.0.0.1", 9000)
	addr4 := NewAddress("192.168.1.1", 8080)

	if !addr1.Equal(addr2) {
		t.Error("相同地址应相等")
	}

	if addr1.Equal(addr3) {
		t.Error("不同端口的地址不应相等")
	}

	if addr1.Equal(addr4) {
		t.Error("不同主机的地址不应相等")
	}

	if addr1.Equal(nil) {
		t.Error("与 nil 比较应返回 false")
	}
}

// TestAddressIsPublic 测试公网地址判断
func TestAddressIsPublic(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		isPublic bool
	}{
		{"公网 IP", "8.8.8.8", 53, true},
		{"私网 IP 10.x", "10.0.0.1", 8080, false},
		{"私网 IP 192.168.x", "192.168.1.1", 8080, false},
		{"私网 IP 172.16.x", "172.16.0.1", 8080, false},
		{"回环地址", "127.0.0.1", 8080, false},
		{"域名", "example.com", 80, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := NewAddress(tt.host, tt.port)
			if addr.IsPublic() != tt.isPublic {
				t.Errorf("IsPublic() = %v，期望 %v", addr.IsPublic(), tt.isPublic)
			}
		})
	}
}

// TestAddressIsPrivate 测试私网地址判断
func TestAddressIsPrivate(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		port      int
		isPrivate bool
	}{
		{"私网 IP 10.x", "10.0.0.1", 8080, true},
		{"私网 IP 192.168.x", "192.168.1.1", 8080, true},
		{"私网 IP 172.16.x", "172.16.0.1", 8080, true},
		{"公网 IP", "8.8.8.8", 53, false},
		{"回环地址", "127.0.0.1", 8080, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := NewAddress(tt.host, tt.port)
			if addr.IsPrivate() != tt.isPrivate {
				t.Errorf("IsPrivate() = %v，期望 %v", addr.IsPrivate(), tt.isPrivate)
			}
		})
	}
}

// TestAddressIsLoopback 测试回环地址判断
func TestAddressIsLoopback(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		port       int
		isLoopback bool
	}{
		{"回环 IPv4", "127.0.0.1", 8080, true},
		{"回环 IPv6", "::1", 8080, true},
		{"localhost 域名", "localhost", 8080, true},
		{"公网 IP", "8.8.8.8", 53, false},
		{"私网 IP", "192.168.1.1", 8080, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := NewAddress(tt.host, tt.port)
			if addr.IsLoopback() != tt.isLoopback {
				t.Errorf("IsLoopback() = %v，期望 %v", addr.IsLoopback(), tt.isLoopback)
			}
		})
	}
}

// TestAddressBytes 测试地址字节表示
func TestAddressBytes(t *testing.T) {
	addr := NewAddress("127.0.0.1", 8080)
	bytes := addr.Bytes()

	if string(bytes) != "127.0.0.1:8080" {
		t.Errorf("Bytes() = %s，期望 127.0.0.1:8080", string(bytes))
	}
}

// TestToUDPAddr 测试转换为 UDP 地址
func TestToUDPAddr(t *testing.T) {
	// IPv4
	addr := NewAddress("127.0.0.1", 8080)
	udpAddr, err := addr.ToUDPAddr()
	if err != nil {
		t.Fatalf("转换为 UDP 地址失败: %v", err)
	}
	if udpAddr.Port != 8080 {
		t.Errorf("端口 = %d，期望 8080", udpAddr.Port)
	}

	// IPv6
	addr = NewAddress("::1", 9000)
	udpAddr, err = addr.ToUDPAddr()
	if err != nil {
		t.Fatalf("转换 IPv6 为 UDP 地址失败: %v", err)
	}
	if udpAddr.Port != 9000 {
		t.Errorf("端口 = %d，期望 9000", udpAddr.Port)
	}
}

// TestMultiaddr 测试多地址格式
func TestMultiaddr(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		port      int
		multiaddr string
	}{
		{
			name:      "IPv4",
			host:      "127.0.0.1",
			port:      8080,
			multiaddr: "/ip4/127.0.0.1/udp/8080/quic-v1",
		},
		{
			name:      "IPv6",
			host:      "::1",
			port:      9000,
			multiaddr: "/ip6/::1/udp/9000/quic-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := NewAddress(tt.host, tt.port)
			if addr.Multiaddr() != tt.multiaddr {
				t.Errorf("Multiaddr() = %s，期望 %s", addr.Multiaddr(), tt.multiaddr)
			}
		})
	}
}

// TestParseMultiaddr 测试解析多地址
func TestParseMultiaddr(t *testing.T) {
	tests := []struct {
		name        string
		multiaddr   string
		expectError bool
		host        string
		port        int
	}{
		{
			name:        "有效的 IPv4 多地址",
			multiaddr:   "/ip4/127.0.0.1/udp/8080/quic-v1",
			expectError: false,
			host:        "127.0.0.1",
			port:        8080,
		},
		{
			name:        "有效的 IPv6 多地址",
			multiaddr:   "/ip6/::1/udp/9000/quic-v1",
			expectError: false,
			host:        "::1",
			port:        9000,
		},
		{
			name:        "无效格式 - 太短",
			multiaddr:   "/ip4/127.0.0.1",
			expectError: true,
		},
		{
			name:        "无效格式 - 不以 / 开头",
			multiaddr:   "ip4/127.0.0.1/udp/8080/quic-v1",
			expectError: true,
		},
		{
			name:        "不支持的网络类型",
			multiaddr:   "/tcp/127.0.0.1/8080",
			expectError: true,
		},
		{
			name:        "不是 UDP",
			multiaddr:   "/ip4/127.0.0.1/tcp/8080/quic-v1",
			expectError: true,
		},
		{
			name:        "无效端口",
			multiaddr:   "/ip4/127.0.0.1/udp/invalid/quic-v1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := ParseMultiaddr(tt.multiaddr)

			if tt.expectError {
				if err == nil {
					t.Error("期望返回错误")
				}
				return
			}

			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}

			if addr.Host() != tt.host {
				t.Errorf("Host() = %s，期望 %s", addr.Host(), tt.host)
			}

			if addr.Port() != tt.port {
				t.Errorf("Port() = %d，期望 %d", addr.Port(), tt.port)
			}
		})
	}
}

// TestResolveAddresses 测试地址解析
func TestResolveAddresses(t *testing.T) {
	// 具体地址
	addr := NewAddress("127.0.0.1", 8080)
	addrs, err := ResolveAddresses(addr)
	if err != nil {
		t.Fatalf("解析地址失败: %v", err)
	}
	if len(addrs) != 1 {
		t.Errorf("应返回 1 个地址，实际返回 %d 个", len(addrs))
	}

	// 通配符地址 (这个测试可能在某些环境中失败)
	wildcardAddr := NewAddress("0.0.0.0", 8080)
	addrs, err = ResolveAddresses(wildcardAddr)
	if err != nil {
		t.Fatalf("解析通配符地址失败: %v", err)
	}
	// 至少应该有一个本地地址
	t.Logf("解析到 %d 个本地地址", len(addrs))
}

// TestToNetAddr 测试转换为 net.Addr
func TestToNetAddr(t *testing.T) {
	addr := NewAddress("127.0.0.1", 8080)
	netAddr := addr.ToNetAddr()
	if netAddr == nil {
		t.Fatal("ToNetAddr() 返回 nil")
	}

	if netAddr.String() != "127.0.0.1:8080" {
		t.Errorf("String() = %s，期望 127.0.0.1:8080", netAddr.String())
	}
}

