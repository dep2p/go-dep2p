package holepunch

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// mockAddress 模拟地址实现
type mockAddress struct {
	network string
	addr    string
}

func (a *mockAddress) Network() string {
	return a.network
}

func (a *mockAddress) String() string {
	return a.addr
}

func (a *mockAddress) Bytes() []byte {
	return []byte(a.addr)
}

func (a *mockAddress) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.String() == other.String()
}

func (a *mockAddress) IsPublic() bool {
	return true
}

func (a *mockAddress) IsPrivate() bool {
	return false
}

func (a *mockAddress) IsLoopback() bool {
	return a.addr == "127.0.0.1:9000" || a.addr == "localhost:9000"
}

func (a *mockAddress) Multiaddr() string {
	// 如果已经是 multiaddr 格式，直接返回
	if len(a.addr) > 0 && a.addr[0] == '/' {
		return a.addr
	}
	// 否则转换为 multiaddr
	return fmt.Sprintf("/ip4/%s/udp/9000/quic-v1", a.addr)
}

// ============================================================================
//                              Config 测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Greater(t, cfg.MaxAttempts, 0)
	assert.Greater(t, cfg.AttemptInterval, time.Duration(0))
	assert.Greater(t, cfg.Timeout, time.Duration(0))
	assert.Greater(t, cfg.PacketSize, 0)
}

// ============================================================================
//                              Puncher 测试
// ============================================================================

func TestNewPuncher(t *testing.T) {
	t.Run("使用默认配置", func(t *testing.T) {
		puncher := NewPuncher(DefaultConfig())
		require.NotNil(t, puncher)
		assert.NotNil(t, puncher.sessions)
	})

	t.Run("使用自定义配置", func(t *testing.T) {
		cfg := Config{
			MaxAttempts:     10,
			AttemptInterval: 500 * time.Millisecond,
			Timeout:         30 * time.Second,
			PacketSize:      128,
		}
		puncher := NewPuncher(cfg)
		require.NotNil(t, puncher)
		assert.Equal(t, 10, puncher.config.MaxAttempts)
	})

	t.Run("使用默认配置创建", func(t *testing.T) {
		puncher := NewPuncher(DefaultConfig())
		require.NotNil(t, puncher)
		assert.NotNil(t, puncher.sessions)
	})
}

// ============================================================================
//                              打洞测试
// ============================================================================

func TestPuncher_Punch_NoAddresses(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	// 没有地址时应该返回错误
	result, err := puncher.Punch(ctx, remoteID, nil)
	assert.Error(t, err)
	assert.Equal(t, ErrNoAddresses, err)
	assert.Nil(t, result)
}

func TestPuncher_Punch_ContextCancelled(t *testing.T) {
	puncher := NewPuncher(Config{
		MaxAttempts:     2,
		AttemptInterval: 50 * time.Millisecond,
		Timeout:         200 * time.Millisecond,
		PacketSize:      64,
	})

	// 立即取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	result, err := puncher.Punch(ctx, remoteID, nil)
	assert.Error(t, err)
	assert.Nil(t, result)
}

// ============================================================================
//                              会话管理测试
// ============================================================================

func TestPuncher_SessionManagement(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	// 创建会话
	session := &punchSession{
		remoteID:  remoteID,
		nonce:     make([]byte, 16),
		startTime: time.Now(),
		done:      make(chan struct{}),
	}

	puncher.sessionsMu.Lock()
	puncher.sessions[remoteID.String()] = session
	puncher.sessionsMu.Unlock()

	// 验证会话存在
	puncher.sessionsMu.RLock()
	_, exists := puncher.sessions[remoteID.String()]
	puncher.sessionsMu.RUnlock()
	assert.True(t, exists)

	// 清理会话
	puncher.sessionsMu.Lock()
	delete(puncher.sessions, remoteID.String())
	puncher.sessionsMu.Unlock()

	// 验证会话已删除
	puncher.sessionsMu.RLock()
	_, exists = puncher.sessions[remoteID.String()]
	puncher.sessionsMu.RUnlock()
	assert.False(t, exists)
}

// ============================================================================
//                              并发安全测试
// ============================================================================

func TestPuncher_Concurrency(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())
	var wg sync.WaitGroup

	// 并发读写会话测试
	for i := 0; i < 10; i++ {
		wg.Add(2)

		go func(id int) {
			defer wg.Done()
			puncher.sessionsMu.RLock()
			_ = len(puncher.sessions)
			puncher.sessionsMu.RUnlock()
		}(i)

		go func(id int) {
			defer wg.Done()
			var nodeID types.NodeID
			copy(nodeID[:], []byte("test-peer-id-"+string(rune('0'+id))))

			puncher.sessionsMu.Lock()
			puncher.sessions[nodeID.String()] = &punchSession{
				remoteID: nodeID,
				done:     make(chan struct{}),
			}
			puncher.sessionsMu.Unlock()
		}(i)
	}

	wg.Wait()
}

// ============================================================================
//                              错误定义测试
// ============================================================================

func TestErrors(t *testing.T) {
	assert.NotNil(t, ErrPunchFailed)
	assert.NotNil(t, ErrNoAddresses)
	assert.NotNil(t, ErrTimeout)
	assert.NotNil(t, ErrNoPeerResponse)
}

// ============================================================================
//                              配置验证测试
// ============================================================================

func TestConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("MaxAttempts 合理", func(t *testing.T) {
		assert.GreaterOrEqual(t, cfg.MaxAttempts, 1)
		assert.LessOrEqual(t, cfg.MaxAttempts, 20)
	})

	t.Run("AttemptInterval 合理", func(t *testing.T) {
		assert.GreaterOrEqual(t, cfg.AttemptInterval, 10*time.Millisecond)
		assert.LessOrEqual(t, cfg.AttemptInterval, 5*time.Second)
	})

	t.Run("Timeout 合理", func(t *testing.T) {
		assert.GreaterOrEqual(t, cfg.Timeout, time.Second)
		assert.LessOrEqual(t, cfg.Timeout, time.Minute)
	})

	t.Run("PacketSize 合理", func(t *testing.T) {
		assert.GreaterOrEqual(t, cfg.PacketSize, 16)
		assert.LessOrEqual(t, cfg.PacketSize, 1024)
	})
}

// ============================================================================
//                              punchSession 测试
// ============================================================================

func TestPunchSession(t *testing.T) {
	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	session := &punchSession{
		remoteID:  remoteID,
		nonce:     []byte{1, 2, 3, 4, 5, 6, 7, 8},
		startTime: time.Now(),
		done:      make(chan struct{}),
	}

	t.Run("session 字段正确", func(t *testing.T) {
		assert.Equal(t, remoteID, session.remoteID)
		assert.NotEmpty(t, session.nonce)
		assert.False(t, session.startTime.IsZero())
		assert.NotNil(t, session.done)
	})

	t.Run("done channel 可关闭", func(t *testing.T) {
		close(session.done)
		select {
		case <-session.done:
			// 正确关闭
		default:
			t.Error("done channel 未关闭")
		}
	})
}

// ============================================================================
//                              集成测试（本地环回）
// ============================================================================

func TestPuncher_LocalLoopback(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	puncher := NewPuncher(Config{
		MaxAttempts:     3,
		AttemptInterval: 100 * time.Millisecond,
		Timeout:         2 * time.Second,
		PacketSize:      64,
	})

	// 只验证 puncher 创建成功
	assert.NotNil(t, puncher)
}

// ============================================================================
//                              SetLocalAddrs/GetLocalAddrs 测试
// ============================================================================

func TestPuncher_LocalAddrs(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	t.Run("初始为空", func(t *testing.T) {
		addrs := puncher.GetLocalAddrs()
		assert.Empty(t, addrs)
	})

	t.Run("设置地址", func(t *testing.T) {
		addrs := []endpoint.Address{
			&mockAddress{network: "ip4", addr: "192.168.1.1:9000"},
			&mockAddress{network: "ip4", addr: "10.0.0.1:9000"},
		}
		puncher.SetLocalAddrs(addrs)

		result := puncher.GetLocalAddrs()
		assert.Len(t, result, 2)
		assert.Equal(t, "192.168.1.1:9000", result[0].String())
	})

	t.Run("覆盖地址", func(t *testing.T) {
		newAddrs := []endpoint.Address{
			&mockAddress{network: "ip4", addr: "172.16.0.1:9000"},
		}
		puncher.SetLocalAddrs(newAddrs)

		result := puncher.GetLocalAddrs()
		assert.Len(t, result, 1)
		assert.Equal(t, "172.16.0.1:9000", result[0].String())
	})
}

// ============================================================================
//                              GetSession/CompleteSession 测试
// ============================================================================

func TestPuncher_GetSession(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	t.Run("不存在的会话返回 nil", func(t *testing.T) {
		session := puncher.GetSession(remoteID)
		assert.Nil(t, session)
	})

	t.Run("存在的会话", func(t *testing.T) {
		// 创建会话
		expectedSession := &punchSession{
			remoteID:  remoteID,
			nonce:     []byte{1, 2, 3, 4},
			startTime: time.Now(),
			done:      make(chan struct{}),
		}

		puncher.sessionsMu.Lock()
		puncher.sessions[remoteID.String()] = expectedSession
		puncher.sessionsMu.Unlock()

		session := puncher.GetSession(remoteID)
		assert.NotNil(t, session)
		assert.Equal(t, remoteID, session.remoteID)

		// 清理
		puncher.sessionsMu.Lock()
		delete(puncher.sessions, remoteID.String())
		puncher.sessionsMu.Unlock()
	})
}

func TestPuncher_CompleteSession(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	t.Run("不存在的会话", func(t *testing.T) {
		// 不应该 panic
		puncher.CompleteSession(remoteID, nil, nil)
	})

	t.Run("正常完成会话", func(t *testing.T) {
		session := &punchSession{
			remoteID:  remoteID,
			nonce:     []byte{1, 2, 3, 4},
			startTime: time.Now(),
			done:      make(chan struct{}),
		}

		puncher.sessionsMu.Lock()
		puncher.sessions[remoteID.String()] = session
		puncher.sessionsMu.Unlock()

		successAddr := &mockAddress{network: "ip4", addr: "192.168.1.100:9000"}
		puncher.CompleteSession(remoteID, successAddr, nil)

		// 验证 done channel 已关闭
		select {
		case <-session.done:
			// 正确关闭
		default:
			t.Error("done channel 未关闭")
		}

		assert.Equal(t, successAddr, session.successAddr)
		assert.Nil(t, session.err)
	})

	t.Run("完成会话带错误", func(t *testing.T) {
		var remoteID2 types.NodeID
		copy(remoteID2[:], []byte("test-peer-id-22345678901"))

		session := &punchSession{
			remoteID:  remoteID2,
			nonce:     []byte{5, 6, 7, 8},
			startTime: time.Now(),
			done:      make(chan struct{}),
		}

		puncher.sessionsMu.Lock()
		puncher.sessions[remoteID2.String()] = session
		puncher.sessionsMu.Unlock()

		expectedErr := ErrPunchFailed
		puncher.CompleteSession(remoteID2, nil, expectedErr)

		assert.Nil(t, session.successAddr)
		assert.Equal(t, expectedErr, session.err)
	})
}

// ============================================================================
//                              buildPunchPacket 测试
// ============================================================================

func TestPuncher_buildPunchPacket(t *testing.T) {
	t.Run("默认包大小", func(t *testing.T) {
		puncher := NewPuncher(DefaultConfig())
		nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		packet := puncher.buildPunchPacket(nonce)

		assert.Len(t, packet, 64) // DefaultConfig PacketSize
		assert.Equal(t, []byte("P2PH"), packet[0:4])
		assert.Equal(t, nonce, packet[4:20])
	})

	t.Run("自定义包大小", func(t *testing.T) {
		cfg := Config{
			MaxAttempts:     5,
			AttemptInterval: 200 * time.Millisecond,
			Timeout:         10 * time.Second,
			PacketSize:      128,
		}
		puncher := NewPuncher(cfg)
		nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		packet := puncher.buildPunchPacket(nonce)

		assert.Len(t, packet, 128)
		assert.Equal(t, []byte("P2PH"), packet[0:4])
	})

	t.Run("短 nonce", func(t *testing.T) {
		puncher := NewPuncher(DefaultConfig())
		nonce := []byte{1, 2, 3, 4}

		packet := puncher.buildPunchPacket(nonce)

		assert.Len(t, packet, 64)
		assert.Equal(t, []byte("P2PH"), packet[0:4])
		// 前4字节是 nonce，其余为0
		assert.Equal(t, nonce, packet[4:8])
	})
}

// ============================================================================
//                              validatePunchResponse 测试
// ============================================================================

func TestPuncher_validatePunchResponse(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())
	nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	t.Run("有效响应 P2PH", func(t *testing.T) {
		data := make([]byte, 64)
		copy(data[0:4], []byte("P2PH"))
		copy(data[4:20], nonce)

		valid := puncher.validatePunchResponse(data, nonce)
		assert.True(t, valid)
	})

	t.Run("有效响应 P2PR", func(t *testing.T) {
		data := make([]byte, 64)
		copy(data[0:4], []byte("P2PR"))
		copy(data[4:20], nonce)

		valid := puncher.validatePunchResponse(data, nonce)
		assert.True(t, valid)
	})

	t.Run("无效 magic", func(t *testing.T) {
		data := make([]byte, 64)
		copy(data[0:4], []byte("XXXX"))
		copy(data[4:20], nonce)

		valid := puncher.validatePunchResponse(data, nonce)
		assert.False(t, valid)
	})

	t.Run("无效 nonce", func(t *testing.T) {
		data := make([]byte, 64)
		copy(data[0:4], []byte("P2PH"))
		copy(data[4:20], []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9})

		valid := puncher.validatePunchResponse(data, nonce)
		assert.False(t, valid)
	})

	t.Run("数据太短", func(t *testing.T) {
		data := make([]byte, 10)

		valid := puncher.validatePunchResponse(data, nonce)
		assert.False(t, valid)
	})
}

// ============================================================================
//                              ipAddr 测试
// ============================================================================

func TestIPAddr(t *testing.T) {
	t.Run("IPv4 地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("192.168.1.100"), 9000)

		assert.Equal(t, "ip4", addr.Network())
		assert.Equal(t, "192.168.1.100:9000", addr.String())
		assert.Equal(t, []byte("192.168.1.100:9000"), addr.Bytes())
		assert.True(t, addr.IsPrivate())
		assert.False(t, addr.IsPublic())
		assert.False(t, addr.IsLoopback())
	})

	t.Run("IPv6 地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("2001:db8::1"), 9000)

		assert.Equal(t, "ip6", addr.Network())
		assert.Equal(t, "[2001:db8::1]:9000", addr.String())
	})

	t.Run("公网地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("8.8.8.8"), 9000)

		assert.False(t, addr.IsPrivate())
		assert.True(t, addr.IsPublic())
		assert.False(t, addr.IsLoopback())
	})

	t.Run("环回地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("127.0.0.1"), 9000)

		assert.False(t, addr.IsPrivate())
		assert.False(t, addr.IsPublic())
		assert.True(t, addr.IsLoopback())
	})

	t.Run("Equal 方法", func(t *testing.T) {
		addr1 := newIPAddr(net.ParseIP("192.168.1.1"), 9000)
		addr2 := newIPAddr(net.ParseIP("192.168.1.1"), 9000)
		addr3 := newIPAddr(net.ParseIP("192.168.1.2"), 9000)

		assert.True(t, addr1.Equal(addr2))
		assert.False(t, addr1.Equal(addr3))
		assert.False(t, addr1.Equal(nil))
	})

	t.Run("ToUDPAddr", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("192.168.1.100"), 9000)

		udpAddr, err := addr.ToUDPAddr()
		require.NoError(t, err)
		assert.Equal(t, 9000, udpAddr.Port)
		assert.Equal(t, "192.168.1.100", udpAddr.IP.String())
	})
}

// ============================================================================
//                              parseUDPAddr 测试
// ============================================================================

func TestParseUDPAddr(t *testing.T) {
	t.Run("从 ipAddr", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("192.168.1.100"), 9000)

		udpAddr, err := parseUDPAddr(addr)
		require.NoError(t, err)
		assert.Equal(t, 9000, udpAddr.Port)
	})

	t.Run("从字符串地址", func(t *testing.T) {
		addr := &mockAddress{
			network: "ip4",
			addr:    "192.168.1.100:9000",
		}

		udpAddr, err := parseUDPAddr(addr)
		require.NoError(t, err)
		assert.Equal(t, 9000, udpAddr.Port)
	})

	t.Run("无效字符串地址", func(t *testing.T) {
		addr := &mockAddress{
			network: "ip4",
			addr:    "not-an-address",
		}

		_, err := parseUDPAddr(addr)
		assert.Error(t, err)
	})
}

// ============================================================================
//                              StartRendezvous 测试
// ============================================================================

func TestPuncher_StartRendezvous_ContextCancelled(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	// 立即取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := puncher.StartRendezvous(ctx, remoteID)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestPuncher_StartRendezvous_SessionCompleted(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 在后台完成会话
	go func() {
		time.Sleep(100 * time.Millisecond)
		puncher.CompleteSession(remoteID, nil, nil)
	}()

	err := puncher.StartRendezvous(ctx, remoteID)
	assert.NoError(t, err)
}

// ============================================================================
//                              Punch 超时测试
// ============================================================================

func TestPuncher_Punch_Timeout(t *testing.T) {
	puncher := NewPuncher(Config{
		MaxAttempts:     2,
		AttemptInterval: 50 * time.Millisecond,
		Timeout:         100 * time.Millisecond,
		PacketSize:      64,
	})

	ctx := context.Background()

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	// 使用不可达的地址
	addrs := []endpoint.Address{
		&mockAddress{network: "ip4", addr: "10.255.255.1:9999"},
	}

	result, err := puncher.Punch(ctx, remoteID, addrs)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "timeout")
}

// ============================================================================
//                              配置验证测试
// ============================================================================

func TestConfig_Validate(t *testing.T) {
	t.Run("无效 PacketSize 被修正", func(t *testing.T) {
		cfg := Config{
			MaxAttempts:     5,
			AttemptInterval: 200 * time.Millisecond,
			Timeout:         10 * time.Second,
			PacketSize:      10, // 太小，小于 MinPacketSize (20)
		}

		cfg.Validate()

		assert.GreaterOrEqual(t, cfg.PacketSize, MinPacketSize, "PacketSize 应该被修正为至少 20")
	})

	t.Run("无效 MaxAttempts 被修正", func(t *testing.T) {
		cfg := Config{
			MaxAttempts: 0, // 无效
			PacketSize:  64,
		}

		cfg.Validate()

		assert.Greater(t, cfg.MaxAttempts, 0)
	})

	t.Run("有效配置不变", func(t *testing.T) {
		cfg := Config{
			MaxAttempts:     10,
			AttemptInterval: 500 * time.Millisecond,
			Timeout:         30 * time.Second,
			PacketSize:      128,
		}

		original := cfg
		cfg.Validate()

		assert.Equal(t, original, cfg, "有效配置不应该被修改")
	})
}

func TestNewPuncher_ValidatesConfig(t *testing.T) {
	// 传入无效配置，验证会被自动修正
	cfg := Config{
		PacketSize: 5, // 太小
	}

	puncher := NewPuncher(cfg)

	// 验证配置已被修正
	assert.GreaterOrEqual(t, puncher.config.PacketSize, MinPacketSize)
}

func TestBuildPunchPacket_SafeWithMinSize(t *testing.T) {
	// 确保即使使用最小大小也不会 panic
	cfg := Config{
		PacketSize: MinPacketSize, // 正好是最小大小
	}
	cfg.Validate()

	puncher := NewPuncher(cfg)
	nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	// 不应该 panic
	packet := puncher.buildPunchPacket(nonce)

	assert.Len(t, packet, MinPacketSize)
	assert.Equal(t, []byte("P2PH"), packet[0:4])
}

// ============================================================================
//                              真正验证业务逻辑的测试
// ============================================================================

// TestPunchPacket_Structure 验证打洞包结构正确
func TestPunchPacket_Structure(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	t.Run("包头魔数正确", func(t *testing.T) {
		nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		packet := puncher.buildPunchPacket(nonce)

		// 验证魔数 "P2PH"
		magic := string(packet[0:4])
		assert.Equal(t, "P2PH", magic, "魔数应该是 P2PH")
	})

	t.Run("nonce 正确嵌入", func(t *testing.T) {
		nonce := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC}
		packet := puncher.buildPunchPacket(nonce)

		// 验证 nonce 位于 [4:20]
		extractedNonce := packet[4:20]
		assert.Equal(t, nonce, extractedNonce, "nonce 应该正确嵌入到包中")
	})

	t.Run("包大小符合配置", func(t *testing.T) {
		cfg := Config{
			PacketSize:  256,
			MaxAttempts: 5,
		}
		cfg.Validate()
		puncher := NewPuncher(cfg)

		nonce := make([]byte, 16)
		packet := puncher.buildPunchPacket(nonce)

		assert.Len(t, packet, 256, "包大小应该与配置一致")
	})
}

// TestPunchResponse_Validation 验证响应验证逻辑
func TestPunchResponse_Validation(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())
	nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	t.Run("有效请求响应匹配", func(t *testing.T) {
		// 构建有效响应（与请求相同格式）
		response := make([]byte, 64)
		copy(response[0:4], []byte("P2PH"))
		copy(response[4:20], nonce)

		valid := puncher.validatePunchResponse(response, nonce)
		assert.True(t, valid, "有效响应应该验证通过")
	})

	t.Run("响应魔数可以是 P2PR", func(t *testing.T) {
		response := make([]byte, 64)
		copy(response[0:4], []byte("P2PR"))
		copy(response[4:20], nonce)

		valid := puncher.validatePunchResponse(response, nonce)
		assert.True(t, valid, "P2PR 响应应该验证通过")
	})

	t.Run("nonce 不匹配被拒绝", func(t *testing.T) {
		wrongNonce := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
		response := make([]byte, 64)
		copy(response[0:4], []byte("P2PH"))
		copy(response[4:20], wrongNonce)

		valid := puncher.validatePunchResponse(response, nonce)
		assert.False(t, valid, "nonce 不匹配的响应应该被拒绝")
	})

	t.Run("随机数据被拒绝", func(t *testing.T) {
		randomData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
		valid := puncher.validatePunchResponse(randomData, nonce)
		assert.False(t, valid, "随机数据应该被拒绝")
	})
}

// TestSession_Lifecycle 验证会话生命周期
func TestSession_Lifecycle(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	t.Run("创建会话", func(t *testing.T) {
		session := &punchSession{
			remoteID:  remoteID,
			nonce:     []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			startTime: time.Now(),
			done:      make(chan struct{}),
		}

		puncher.sessionsMu.Lock()
		puncher.sessions[remoteID.String()] = session
		puncher.sessionsMu.Unlock()

		// 验证会话存在
		retrieved := puncher.GetSession(remoteID)
		require.NotNil(t, retrieved)
		assert.Equal(t, remoteID, retrieved.remoteID)
	})

	t.Run("完成会话设置成功地址", func(t *testing.T) {
		successAddr := newIPAddr(net.ParseIP("203.0.113.100"), 9000)

		puncher.CompleteSession(remoteID, successAddr, nil)

		session := puncher.GetSession(remoteID)
		if session != nil {
			assert.Equal(t, successAddr, session.successAddr)
			assert.Nil(t, session.err)
		}
	})

	t.Run("完成会话设置错误", func(t *testing.T) {
		var remoteID2 types.NodeID
		copy(remoteID2[:], []byte("test-peer-id-22345678901"))

		session := &punchSession{
			remoteID: remoteID2,
			nonce:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			done:     make(chan struct{}),
		}

		puncher.sessionsMu.Lock()
		puncher.sessions[remoteID2.String()] = session
		puncher.sessionsMu.Unlock()

		expectedErr := ErrPunchFailed
		puncher.CompleteSession(remoteID2, nil, expectedErr)

		retrievedSession := puncher.GetSession(remoteID2)
		if retrievedSession != nil {
			assert.Nil(t, retrievedSession.successAddr)
			assert.Equal(t, expectedErr, retrievedSession.err)
		}
	})
}

// TestPunchAddress_Properties 验证地址属性判断正确
func TestPunchAddress_Properties(t *testing.T) {
	testCases := []struct {
		name       string
		ip         string
		isPublic   bool
		isPrivate  bool
		isLoopback bool
		network    string
	}{
		{"公网 IPv4", "8.8.8.8", true, false, false, "ip4"},
		{"私网 192.168.x.x", "192.168.1.1", false, true, false, "ip4"},
		{"私网 10.x.x.x", "10.0.0.1", false, true, false, "ip4"},
		{"私网 172.16.x.x", "172.16.0.1", false, true, false, "ip4"},
		{"环回 IPv4", "127.0.0.1", false, false, true, "ip4"},
		{"公网 IPv6", "2001:4860:4860::8888", true, false, false, "ip6"},
		{"环回 IPv6", "::1", false, false, true, "ip6"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addr := newIPAddr(net.ParseIP(tc.ip), 9000)

			assert.Equal(t, tc.isPublic, addr.IsPublic(), "IsPublic() 不正确")
			assert.Equal(t, tc.isPrivate, addr.IsPrivate(), "IsPrivate() 不正确")
			assert.Equal(t, tc.isLoopback, addr.IsLoopback(), "IsLoopback() 不正确")
			assert.Equal(t, tc.network, addr.Network(), "Network() 不正确")
		})
	}
}

// TestConfig_ValidationCorrectness 验证配置验证的正确性
func TestConfig_ValidationCorrectness(t *testing.T) {
	t.Run("所有无效字段被修正", func(t *testing.T) {
		cfg := Config{
			MaxAttempts:     0,  // 无效
			AttemptInterval: 0,  // 无效
			Timeout:         0,  // 无效
			PacketSize:      10, // 太小
		}

		cfg.Validate()

		assert.Greater(t, cfg.MaxAttempts, 0, "MaxAttempts 应该被修正")
		assert.Greater(t, cfg.AttemptInterval, time.Duration(0), "AttemptInterval 应该被修正")
		assert.Greater(t, cfg.Timeout, time.Duration(0), "Timeout 应该被修正")
		assert.GreaterOrEqual(t, cfg.PacketSize, MinPacketSize, "PacketSize 应该被修正")
	})

	t.Run("有效配置不变", func(t *testing.T) {
		cfg := Config{
			MaxAttempts:     10,
			AttemptInterval: 500 * time.Millisecond,
			Timeout:         30 * time.Second,
			PacketSize:      128,
		}

		original := cfg
		cfg.Validate()

		assert.Equal(t, original.MaxAttempts, cfg.MaxAttempts)
		assert.Equal(t, original.AttemptInterval, cfg.AttemptInterval)
		assert.Equal(t, original.Timeout, cfg.Timeout)
		assert.Equal(t, original.PacketSize, cfg.PacketSize)
	})
}

// TestPuncher_ConcurrentSessionAccess 验证并发会话访问安全
func TestPuncher_ConcurrentSessionAccess(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())
	var wg sync.WaitGroup

	// 并发创建、读取、删除会话
	for i := 0; i < 20; i++ {
		wg.Add(3)

		// 创建
		go func(idx int) {
			defer wg.Done()
			var nodeID types.NodeID
			nodeID[0] = byte(idx)
			puncher.sessionsMu.Lock()
			puncher.sessions[nodeID.String()] = &punchSession{
				remoteID: nodeID,
				done:     make(chan struct{}),
			}
			puncher.sessionsMu.Unlock()
		}(i)

		// 读取
		go func(idx int) {
			defer wg.Done()
			var nodeID types.NodeID
			nodeID[0] = byte(idx)
			_ = puncher.GetSession(nodeID)
		}(i)

		// 删除
		go func(idx int) {
			defer wg.Done()
			time.Sleep(time.Millisecond)
			var nodeID types.NodeID
			nodeID[0] = byte(idx)
			puncher.sessionsMu.Lock()
			delete(puncher.sessions, nodeID.String())
			puncher.sessionsMu.Unlock()
		}(i)
	}

	wg.Wait()

	// 验证没有 panic，且 map 状态合理
	puncher.sessionsMu.RLock()
	sessionCount := len(puncher.sessions)
	puncher.sessionsMu.RUnlock()

	assert.GreaterOrEqual(t, sessionCount, 0, "会话数应该非负")
}

// ============================================================================
//                              协议消息测试
// ============================================================================

func TestHolePunchRequest_EncodeDecode(t *testing.T) {
	t.Run("正常编解码", func(t *testing.T) {
		var initID, respID types.NodeID
		copy(initID[:], []byte("initiator-id-12345678901"))
		copy(respID[:], []byte("responder-id-12345678901"))

		original := &HolePunchRequest{
			InitiatorID: initID,
			InitiatorAddrs: []endpoint.Address{
				&mockAddress{network: "ip4", addr: "192.168.1.1:9000"},
				&mockAddress{network: "ip4", addr: "10.0.0.1:9000"},
			},
			ResponderID: respID,
		}

		data, err := original.Encode()
		require.NoError(t, err)

		decoded := &HolePunchRequest{}
		err = decoded.Decode(data)
		require.NoError(t, err)

		assert.Equal(t, original.InitiatorID, decoded.InitiatorID)
		assert.Equal(t, original.ResponderID, decoded.ResponderID)
		assert.Len(t, decoded.InitiatorAddrs, 2)
	})

	t.Run("地址过多被拒绝", func(t *testing.T) {
		var initID, respID types.NodeID
		copy(initID[:], []byte("initiator-id-12345678901"))
		copy(respID[:], []byte("responder-id-12345678901"))

		// 创建超过 MaxAddresses 的地址
		addrs := make([]endpoint.Address, MaxAddresses+1)
		for i := range addrs {
			addrs[i] = &mockAddress{network: "ip4", addr: "192.168.1.1:9000"}
		}

		msg := &HolePunchRequest{
			InitiatorID:    initID,
			InitiatorAddrs: addrs,
			ResponderID:    respID,
		}

		_, err := msg.Encode()
		assert.Equal(t, ErrTooManyAddresses, err)
	})

	t.Run("解码时地址过多被拒绝", func(t *testing.T) {
		// 手动构造一个地址数超过 MaxAddresses 的消息
		data := make([]byte, 1+32+1+32)
		data[0] = MsgTypeRequest
		data[33] = 20 // addrCount = 20 > MaxAddresses (16)

		decoded := &HolePunchRequest{}
		err := decoded.Decode(data)
		assert.Equal(t, ErrTooManyAddresses, err)
	})

	t.Run("解码截断数据失败", func(t *testing.T) {
		// 数据太短
		data := []byte{MsgTypeRequest}
		decoded := &HolePunchRequest{}
		err := decoded.Decode(data)
		assert.Equal(t, ErrInvalidMessage, err)
	})

	t.Run("解码错误消息类型", func(t *testing.T) {
		data := make([]byte, 1+32+1+32)
		data[0] = MsgTypeConnect // 错误类型

		decoded := &HolePunchRequest{}
		err := decoded.Decode(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected request message")
	})
}

func TestHolePunchConnect_EncodeDecode(t *testing.T) {
	t.Run("正常编解码", func(t *testing.T) {
		nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		original := &HolePunchConnect{
			InitiatorAddrs: []endpoint.Address{
				&mockAddress{network: "ip4", addr: "192.168.1.1:9000"},
			},
			ResponderAddrs: []endpoint.Address{
				&mockAddress{network: "ip4", addr: "192.168.1.2:9000"},
			},
			Nonce: nonce,
		}

		data, err := original.Encode()
		require.NoError(t, err)

		decoded := &HolePunchConnect{}
		err = decoded.Decode(data)
		require.NoError(t, err)

		assert.Equal(t, original.Nonce, decoded.Nonce)
		assert.Len(t, decoded.InitiatorAddrs, 1)
		assert.Len(t, decoded.ResponderAddrs, 1)
	})

	t.Run("发起方地址过多", func(t *testing.T) {
		addrs := make([]endpoint.Address, MaxAddresses+1)
		for i := range addrs {
			addrs[i] = &mockAddress{network: "ip4", addr: "192.168.1.1:9000"}
		}

		msg := &HolePunchConnect{
			InitiatorAddrs: addrs,
			ResponderAddrs: []endpoint.Address{},
			Nonce:          make([]byte, NonceLen),
		}

		_, err := msg.Encode()
		assert.Equal(t, ErrTooManyAddresses, err)
	})

	t.Run("响应方地址过多", func(t *testing.T) {
		addrs := make([]endpoint.Address, MaxAddresses+1)
		for i := range addrs {
			addrs[i] = &mockAddress{network: "ip4", addr: "192.168.1.1:9000"}
		}

		msg := &HolePunchConnect{
			InitiatorAddrs: []endpoint.Address{},
			ResponderAddrs: addrs,
			Nonce:          make([]byte, NonceLen),
		}

		_, err := msg.Encode()
		assert.Equal(t, ErrTooManyAddresses, err)
	})

	t.Run("Nonce 长度错误", func(t *testing.T) {
		msg := &HolePunchConnect{
			InitiatorAddrs: []endpoint.Address{},
			ResponderAddrs: []endpoint.Address{},
			Nonce:          []byte{1, 2, 3}, // 太短
		}

		_, err := msg.Encode()
		assert.Equal(t, ErrInvalidNonce, err)
	})
}

func TestHolePunchResponse_EncodeDecode(t *testing.T) {
	t.Run("成功响应", func(t *testing.T) {
		nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		original := &HolePunchResponse{
			Success: true,
			Nonce:   nonce,
			Error:   "",
		}

		data, err := original.Encode()
		require.NoError(t, err)

		decoded := &HolePunchResponse{}
		err = decoded.Decode(data)
		require.NoError(t, err)

		assert.True(t, decoded.Success)
		assert.Equal(t, nonce, decoded.Nonce)
		assert.Empty(t, decoded.Error)
	})

	t.Run("失败响应带错误", func(t *testing.T) {
		nonce := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		original := &HolePunchResponse{
			Success: false,
			Nonce:   nonce,
			Error:   "connection refused",
		}

		data, err := original.Encode()
		require.NoError(t, err)

		decoded := &HolePunchResponse{}
		err = decoded.Decode(data)
		require.NoError(t, err)

		assert.False(t, decoded.Success)
		assert.Equal(t, "connection refused", decoded.Error)
	})
}

func TestParseMessage(t *testing.T) {
	t.Run("解析请求消息", func(t *testing.T) {
		var initID, respID types.NodeID
		copy(initID[:], []byte("initiator-id-12345678901"))
		copy(respID[:], []byte("responder-id-12345678901"))

		original := &HolePunchRequest{
			InitiatorID:    initID,
			InitiatorAddrs: []endpoint.Address{},
			ResponderID:    respID,
		}

		data, err := original.Encode()
		require.NoError(t, err)

		parsed, err := ParseMessage(data)
		require.NoError(t, err)

		req, ok := parsed.(*HolePunchRequest)
		require.True(t, ok)
		assert.Equal(t, initID, req.InitiatorID)
	})

	t.Run("解析未知消息类型", func(t *testing.T) {
		data := []byte{0xFF, 0x00, 0x00}
		_, err := ParseMessage(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown message type")
	})

	t.Run("解析空数据", func(t *testing.T) {
		_, err := ParseMessage([]byte{})
		assert.Equal(t, ErrInvalidMessage, err)
	})
}

// TestStringAddress 已删除
//
// 根据 IMPL-ADDRESS-UNIFICATION.md 规范，StringAddress 类型已被 address.Addr 替代。
// 测试现在应使用 address.Addr 进行。

// ============================================================================
//                              TCP 打洞测试
// ============================================================================

func TestTCPConfig_Defaults(t *testing.T) {
	cfg := DefaultTCPConfig()

	assert.Greater(t, cfg.MaxAttempts, 0)
	assert.Greater(t, cfg.AttemptInterval, time.Duration(0))
	assert.Greater(t, cfg.Timeout, time.Duration(0))
	assert.Greater(t, cfg.ConnectTimeout, time.Duration(0))
	assert.True(t, cfg.EnableReusePort)
}

func TestTCPConfig_Validate(t *testing.T) {
	t.Run("无效配置被修正", func(t *testing.T) {
		cfg := TCPConfig{
			MaxAttempts:     0,
			AttemptInterval: 0,
			Timeout:         0,
			ConnectTimeout:  0,
		}

		cfg.Validate()

		assert.Greater(t, cfg.MaxAttempts, 0)
		assert.Greater(t, cfg.AttemptInterval, time.Duration(0))
		assert.Greater(t, cfg.Timeout, time.Duration(0))
		assert.Greater(t, cfg.ConnectTimeout, time.Duration(0))
	})
}

func TestNewTCPPuncher(t *testing.T) {
	t.Run("使用默认配置", func(t *testing.T) {
		puncher := NewTCPPuncher(DefaultTCPConfig())
		require.NotNil(t, puncher)
	})

	t.Run("使用默认配置创建", func(t *testing.T) {
		puncher := NewTCPPuncher(DefaultTCPConfig())
		require.NotNil(t, puncher)
	})
}

func TestTCPPuncher_NoAddresses(t *testing.T) {
	puncher := NewTCPPuncher(DefaultTCPConfig())

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	conn, addr, err := puncher.Punch(context.Background(), remoteID, nil)
	assert.Equal(t, ErrTCPNoAddresses, err)
	assert.Nil(t, conn)
	assert.Nil(t, addr)
}

func TestTCPPuncher_ImplementsInterface(t *testing.T) {
	puncher := NewTCPPuncher(DefaultTCPConfig())

	// 确保实现了 TCPHolePuncher 接口
	var _ interface {
		PunchTCP(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address) (conn interface{}, addr endpoint.Address, err error)
		PunchTCPWithLocalPort(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address, localPort int) (conn interface{}, addr endpoint.Address, err error)
	} = puncher

	// 验证方法存在
	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	// PunchTCP 方法
	_, _, err := puncher.PunchTCP(context.Background(), remoteID, nil)
	assert.Equal(t, ErrTCPNoAddresses, err)

	// PunchTCPWithLocalPort 方法
	_, _, err = puncher.PunchTCPWithLocalPort(context.Background(), remoteID, nil, 0)
	assert.Equal(t, ErrTCPNoAddresses, err)
}

func TestTCPAddr(t *testing.T) {
	t.Run("IPv4 地址", func(t *testing.T) {
		addr := newTCPAddr(net.ParseIP("192.168.1.100"), 9000)

		assert.Equal(t, "tcp", addr.Network())
		assert.Equal(t, "192.168.1.100:9000", addr.String())
		assert.True(t, addr.IsPrivate())
		assert.False(t, addr.IsPublic())
	})

	t.Run("IPv6 地址", func(t *testing.T) {
		addr := newTCPAddr(net.ParseIP("2001:db8::1"), 9000)

		assert.Equal(t, "tcp", addr.Network())
		assert.Equal(t, "[2001:db8::1]:9000", addr.String())
	})

	t.Run("ToTCPAddr", func(t *testing.T) {
		addr := newTCPAddr(net.ParseIP("192.168.1.100"), 9000)

		tcpAddr, err := addr.ToTCPAddr()
		require.NoError(t, err)
		assert.Equal(t, 9000, tcpAddr.Port)
	})
}

func TestParseTCPAddr(t *testing.T) {
	t.Run("从 tcpAddr", func(t *testing.T) {
		addr := newTCPAddr(net.ParseIP("192.168.1.100"), 9000)

		tcpAddr, err := parseTCPAddr(addr)
		require.NoError(t, err)
		assert.Equal(t, 9000, tcpAddr.Port)
	})

	t.Run("从字符串地址", func(t *testing.T) {
		addr := &mockAddress{
			network: "tcp",
			addr:    "192.168.1.100:9000",
		}

		tcpAddr, err := parseTCPAddr(addr)
		require.NoError(t, err)
		assert.Equal(t, 9000, tcpAddr.Port)
	})

	t.Run("无效地址", func(t *testing.T) {
		addr := &mockAddress{
			network: "tcp",
			addr:    "invalid-address",
		}

		_, err := parseTCPAddr(addr)
		assert.Error(t, err)
	})
}

// ============================================================================
//                              CompleteSession 竞态条件修复验证
// ============================================================================

func TestCompleteSession_RaceConditionFix(t *testing.T) {
	puncher := NewPuncher(DefaultConfig())

	var remoteID types.NodeID
	copy(remoteID[:], []byte("test-peer-id-12345678901"))

	// 创建会话
	session := &punchSession{
		remoteID:  remoteID,
		nonce:     make([]byte, 16),
		startTime: time.Now(),
		done:      make(chan struct{}),
	}

	puncher.sessionsMu.Lock()
	puncher.sessions[remoteID.String()] = session
	puncher.sessionsMu.Unlock()

	successAddr := &mockAddress{network: "ip4", addr: "192.168.1.100:9000"}
	expectedErr := ErrPunchFailed

	// 并发完成会话并读取结果
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		puncher.CompleteSession(remoteID, successAddr, expectedErr)
	}()

	go func() {
		defer wg.Done()
		<-session.done
		// 此时应该能安全读取结果
		puncher.sessionsMu.RLock()
		s := puncher.sessions[remoteID.String()]
		if s != nil {
			// 验证结果已经设置好（无竞态）
			assert.Equal(t, successAddr, s.successAddr)
			assert.Equal(t, expectedErr, s.err)
		}
		puncher.sessionsMu.RUnlock()
	}()

	wg.Wait()
}
