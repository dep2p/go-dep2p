package connmgr

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGater_New 测试创建 Gater
func TestGater_New(t *testing.T) {
	gater := NewGater()
	require.NotNil(t, gater)

	t.Log("✅ Gater 创建成功")
}

// TestGater_InterceptPeerDial 测试拦截拨号
func TestGater_InterceptPeerDial(t *testing.T) {
	gater := NewGater()

	peer := "test-peer-1"

	// 默认允许
	assert.True(t, gater.InterceptPeerDial(peer))

	// 阻止
	gater.BlockPeer(peer)
	assert.False(t, gater.InterceptPeerDial(peer))

	// 解除阻止
	gater.UnblockPeer(peer)
	assert.True(t, gater.InterceptPeerDial(peer))

	t.Log("✅ InterceptPeerDial 拦截正确")
}

// TestGater_InterceptAddrDial 测试拦截地址拨号
func TestGater_InterceptAddrDial(t *testing.T) {
	gater := NewGater()

	peer := "test-peer-1"
	addr := "/ip4/127.0.0.1/tcp/4001"

	// 默认允许
	assert.True(t, gater.InterceptAddrDial(peer, addr))

	// 阻止节点
	gater.BlockPeer(peer)
	assert.False(t, gater.InterceptAddrDial(peer, addr))

	t.Log("✅ InterceptAddrDial 拦截正确")
}

// TestGater_InterceptAccept 测试拦截入站连接
func TestGater_InterceptAccept(t *testing.T) {
	gater := NewGater()

	// 创建 mock 连接
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 设置远端地址
	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.100/tcp/4001")
	mockConn.RemoteMultiaddrFunc = func() types.Multiaddr {
		return remoteAddr
	}

	// 默认允许
	assert.True(t, gater.InterceptAccept(mockConn))

	t.Log("✅ InterceptAccept 拦截正确")
}

// TestGater_InterceptSecured 测试拦截安全握手
func TestGater_InterceptSecured(t *testing.T) {
	gater := NewGater()

	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("test-peer-1")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 默认允许
	assert.True(t, gater.InterceptSecured(pkgif.DirOutbound, string(remotePeer), mockConn))

	// 阻止节点
	gater.BlockPeer(string(remotePeer))
	assert.False(t, gater.InterceptSecured(pkgif.DirOutbound, string(remotePeer), mockConn))

	// 解除阻止
	gater.UnblockPeer(string(remotePeer))
	assert.True(t, gater.InterceptSecured(pkgif.DirInbound, string(remotePeer), mockConn))

	t.Log("✅ InterceptSecured 拦截正确")
}

// TestGater_InterceptUpgraded 测试拦截升级后连接
func TestGater_InterceptUpgraded(t *testing.T) {
	gater := NewGater()

	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("test-peer-1")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 默认允许
	allow, err := gater.InterceptUpgraded(mockConn)
	assert.NoError(t, err)
	assert.True(t, allow)

	// 阻止节点
	gater.BlockPeer(string(remotePeer))
	allow, err = gater.InterceptUpgraded(mockConn)
	assert.NoError(t, err)
	assert.False(t, allow)

	t.Log("✅ InterceptUpgraded 拦截正确")
}

// TestGater_BlockUnblock 测试阻止和解除阻止
func TestGater_BlockUnblock(t *testing.T) {
	gater := NewGater()

	peer := "test-peer-1"

	// 初始状态：允许
	assert.True(t, gater.InterceptPeerDial(peer))

	// 阻止
	gater.BlockPeer(peer)
	assert.False(t, gater.InterceptPeerDial(peer))

	// 解除阻止
	gater.UnblockPeer(peer)
	assert.True(t, gater.InterceptPeerDial(peer))

	t.Log("✅ 阻止和解除阻止正确")
}

// TestGater_MultipleBlocks 测试多个节点阻止
func TestGater_MultipleBlocks(t *testing.T) {
	gater := NewGater()

	peer1 := "peer-1"
	peer2 := "peer-2"
	peer3 := "peer-3"

	// 阻止 peer1 和 peer2
	gater.BlockPeer(peer1)
	gater.BlockPeer(peer2)

	// 验证
	assert.False(t, gater.InterceptPeerDial(peer1))
	assert.False(t, gater.InterceptPeerDial(peer2))
	assert.True(t, gater.InterceptPeerDial(peer3))

	// 解除 peer1
	gater.UnblockPeer(peer1)
	assert.True(t, gater.InterceptPeerDial(peer1))
	assert.False(t, gater.InterceptPeerDial(peer2))

	t.Log("✅ 多节点阻止正确")
}

// TestGater_Concurrent 测试并发安全
func TestGater_Concurrent(t *testing.T) {
	gater := NewGater()

	done := make(chan bool, 10)

	// 并发阻止和解除阻止
	for i := 0; i < 10; i++ {
		go func(n int) {
			peer := "peer-" + string(rune('0'+n))
			for j := 0; j < 100; j++ {
				gater.BlockPeer(peer)
				gater.InterceptPeerDial(peer)
				gater.UnblockPeer(peer)
			}
			done <- true
		}(i)
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("✅ 并发操作安全")
}

// TestGater_Interface 验证接口实现
func TestGater_Interface(t *testing.T) {
	var _ pkgif.ConnGater = (*Gater)(nil)
	t.Log("✅ Gater 实现 ConnGater 接口")
}

// TestGater_InterceptAccept_NilMultiaddr 测试无远端地址的情况
func TestGater_InterceptAccept_NilMultiaddr(t *testing.T) {
	gater := NewGater()

	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// RemoteMultiaddr 返回 nil（默认行为）
	// 应该允许连接
	assert.True(t, gater.InterceptAccept(mockConn))

	t.Log("✅ 空地址时允许连接")
}

// TestGater_InterceptSecured_Direction 测试不同方向的安全握手
func TestGater_InterceptSecured_Direction(t *testing.T) {
	gater := NewGater()

	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 入站连接
	assert.True(t, gater.InterceptSecured(pkgif.DirInbound, string(remotePeer), mockConn))

	// 出站连接
	assert.True(t, gater.InterceptSecured(pkgif.DirOutbound, string(remotePeer), mockConn))

	// 阻止后，两个方向都被阻止
	gater.BlockPeer(string(remotePeer))
	assert.False(t, gater.InterceptSecured(pkgif.DirInbound, string(remotePeer), mockConn))
	assert.False(t, gater.InterceptSecured(pkgif.DirOutbound, string(remotePeer), mockConn))

	t.Log("✅ 方向性安全握手检查正确")
}

// ============================================================================
// 覆盖率提升测试
// ============================================================================

// TestGater_Clear 测试清空黑名单
func TestGater_Clear(t *testing.T) {
	gater := NewGater()

	// 添加各种黑名单条目
	gater.BlockPeer("peer1")
	gater.BlockPeer("peer2")
	gater.BlockIP("192.168.1.1")
	gater.BlockPort(4001)
	_ = gater.BlockSubnet("10.0.0.0/8")

	// 验证已添加
	assert.True(t, gater.IsBlocked("peer1"))
	assert.Len(t, gater.BlockedPeers(), 2)

	// 清空
	gater.Clear()

	// 验证已清空
	assert.False(t, gater.IsBlocked("peer1"))
	assert.Len(t, gater.BlockedPeers(), 0)
	assert.Len(t, gater.BlockedIPs(), 0)
	assert.Len(t, gater.BlockedSubnets(), 0)
	assert.Len(t, gater.BlockedPortList(), 0)

	t.Log("✅ Clear 功能正确")
}

// TestGater_BlockedPeers 测试获取被阻止节点列表
func TestGater_BlockedPeers(t *testing.T) {
	gater := NewGater()

	// 初始为空
	assert.Len(t, gater.BlockedPeers(), 0)

	// 添加节点
	gater.BlockPeer("peer1")
	gater.BlockPeer("peer2")
	gater.BlockPeer("peer3")

	// 验证
	blocked := gater.BlockedPeers()
	assert.Len(t, blocked, 3)
	assert.Contains(t, blocked, "peer1")
	assert.Contains(t, blocked, "peer2")
	assert.Contains(t, blocked, "peer3")

	t.Log("✅ BlockedPeers 功能正确")
}

// TestGater_BlockIP 测试 IP 黑名单
func TestGater_BlockIP(t *testing.T) {
	gater := NewGater()

	ip := "192.168.1.100"

	// 初始不阻止
	assert.False(t, gater.isIPBlocked(ip))

	// 阻止
	gater.BlockIP(ip)
	assert.True(t, gater.isIPBlocked(ip))

	// 验证列表
	ips := gater.BlockedIPs()
	assert.Contains(t, ips, ip)

	// 解除阻止
	gater.UnblockIP(ip)
	assert.False(t, gater.isIPBlocked(ip))

	t.Log("✅ BlockIP/UnblockIP 功能正确")
}

// TestGater_BlockPort 测试端口黑名单
func TestGater_BlockPort(t *testing.T) {
	gater := NewGater()

	port := 4001

	// 初始不阻止
	assert.False(t, gater.isPortBlocked(port))

	// 阻止
	gater.BlockPort(port)
	assert.True(t, gater.isPortBlocked(port))

	// 验证列表
	ports := gater.BlockedPortList()
	assert.Contains(t, ports, port)

	// 解除阻止
	gater.UnblockPort(port)
	assert.False(t, gater.isPortBlocked(port))

	t.Log("✅ BlockPort/UnblockPort 功能正确")
}

// TestGater_BlockSubnet 测试子网黑名单
func TestGater_BlockSubnet(t *testing.T) {
	gater := NewGater()

	cidr := "10.0.0.0/8"

	// 初始子网内 IP 不阻止
	assert.False(t, gater.isIPBlocked("10.1.2.3"))

	// 阻止子网
	err := gater.BlockSubnet(cidr)
	require.NoError(t, err)

	// 子网内 IP 被阻止
	assert.True(t, gater.isIPBlocked("10.1.2.3"))
	assert.True(t, gater.isIPBlocked("10.255.255.255"))

	// 子网外 IP 不阻止
	assert.False(t, gater.isIPBlocked("192.168.1.1"))

	// 验证列表
	subnets := gater.BlockedSubnets()
	assert.Contains(t, subnets, cidr)

	// 解除阻止
	gater.UnblockSubnet(cidr)
	assert.False(t, gater.isIPBlocked("10.1.2.3"))
	assert.Len(t, gater.BlockedSubnets(), 0)

	t.Log("✅ BlockSubnet/UnblockSubnet 功能正确")
}

// TestGater_BlockSubnet_InvalidCIDR 测试无效 CIDR
func TestGater_BlockSubnet_InvalidCIDR(t *testing.T) {
	gater := NewGater()

	err := gater.BlockSubnet("invalid-cidr")
	assert.Error(t, err)

	t.Log("✅ 无效 CIDR 正确返回错误")
}

// TestGater_Stats 测试统计信息
func TestGater_Stats(t *testing.T) {
	gater := NewGater()

	// 添加各种黑名单
	gater.BlockPeer("peer1")
	gater.BlockPeer("peer2")
	gater.BlockIP("192.168.1.1")
	gater.BlockPort(4001)
	gater.BlockPort(4002)
	_ = gater.BlockSubnet("10.0.0.0/8")

	// 触发拦截
	gater.InterceptPeerDial("peer1")
	gater.InterceptAddrDial("peer2", "/ip4/127.0.0.1/tcp/4001")

	// 获取统计
	stats := gater.Stats()
	assert.Equal(t, 2, stats.BlockedPeers)
	assert.Equal(t, 1, stats.BlockedIPs)
	assert.Equal(t, 2, stats.BlockedPorts)
	assert.Equal(t, 1, stats.BlockedSubnets)
	assert.Greater(t, stats.InterceptedDials, int64(0))

	t.Log("✅ Stats 统计正确")
}

// TestGater_InterceptAddrDial_InvalidAddr 测试无效地址拨号
func TestGater_InterceptAddrDial_InvalidAddr(t *testing.T) {
	gater := NewGater()

	// 无效地址应该被拒绝
	assert.False(t, gater.InterceptAddrDial("peer1", "invalid-addr"))

	t.Log("✅ 无效地址拨号被正确拒绝")
}

// TestGater_InterceptAddrDial_BlockedPort 测试被阻止端口的拨号
func TestGater_InterceptAddrDial_BlockedPort(t *testing.T) {
	gater := NewGater()

	// 阻止端口
	gater.BlockPort(4001)

	// 拨号到被阻止的端口
	assert.False(t, gater.InterceptAddrDial("peer1", "/ip4/127.0.0.1/tcp/4001"))

	// 其他端口允许
	assert.True(t, gater.InterceptAddrDial("peer1", "/ip4/127.0.0.1/tcp/4002"))

	t.Log("✅ 端口阻止拨号正确")
}

// TestGater_InterceptAddrDial_BlockedIP 测试被阻止 IP 的拨号
func TestGater_InterceptAddrDial_BlockedIP(t *testing.T) {
	gater := NewGater()

	// 阻止 IP
	gater.BlockIP("192.168.1.100")

	// 拨号到被阻止的 IP
	assert.False(t, gater.InterceptAddrDial("peer1", "/ip4/192.168.1.100/tcp/4001"))

	// 其他 IP 允许
	assert.True(t, gater.InterceptAddrDial("peer1", "/ip4/192.168.1.101/tcp/4001"))

	t.Log("✅ IP 阻止拨号正确")
}

// TestGater_InterceptAccept_BlockedIP 测试接受时 IP 阻止
func TestGater_InterceptAccept_BlockedIP(t *testing.T) {
	gater := NewGater()

	// 阻止 IP
	gater.BlockIP("192.168.1.100")

	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 设置被阻止的远端地址
	blockedAddr, _ := types.NewMultiaddr("/ip4/192.168.1.100/tcp/4001")
	mockConn.RemoteMultiaddrFunc = func() types.Multiaddr {
		return blockedAddr
	}

	// 应该被拒绝
	assert.False(t, gater.InterceptAccept(mockConn))

	t.Log("✅ 接受时 IP 阻止正确")
}

// TestGater_InterceptAddrDial_IPv6 测试 IPv6 地址拨号
func TestGater_InterceptAddrDial_IPv6(t *testing.T) {
	gater := NewGater()

	// 阻止 IPv6
	gater.BlockIP("::1")

	// 拨号到被阻止的 IPv6
	assert.False(t, gater.InterceptAddrDial("peer1", "/ip6/::1/tcp/4001"))

	// 其他 IPv6 允许
	assert.True(t, gater.InterceptAddrDial("peer1", "/ip6/fe80::1/tcp/4001"))

	t.Log("✅ IPv6 阻止拨号正确")
}

// TestGater_InterceptAccept_IPv6 测试接受时 IPv6 阻止
func TestGater_InterceptAccept_IPv6(t *testing.T) {
	gater := NewGater()

	// 阻止 IPv6
	gater.BlockIP("2001:db8::1")

	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 设置被阻止的远端 IPv6 地址
	blockedAddr, _ := types.NewMultiaddr("/ip6/2001:db8::1/tcp/4001")
	mockConn.RemoteMultiaddrFunc = func() types.Multiaddr {
		return blockedAddr
	}

	// 应该被拒绝
	assert.False(t, gater.InterceptAccept(mockConn))

	t.Log("✅ 接受时 IPv6 阻止正确")
}

// TestGater_IsBlocked 测试 IsBlocked 方法
func TestGater_IsBlocked(t *testing.T) {
	gater := NewGater()

	peer := "test-peer"

	// 初始不阻止
	assert.False(t, gater.IsBlocked(peer))

	// 阻止
	gater.BlockPeer(peer)
	assert.True(t, gater.IsBlocked(peer))

	// 解除阻止
	gater.UnblockPeer(peer)
	assert.False(t, gater.IsBlocked(peer))

	t.Log("✅ IsBlocked 功能正确")
}
