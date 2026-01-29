// Package gater 实现连接门控
package gater

import (
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              Gater 测试
// ============================================================================

func TestGater_New(t *testing.T) {
	g := New()
	require.NotNil(t, g)
	assert.NotNil(t, g.blockedPeers)
	assert.NotNil(t, g.blockedAddrs)
	assert.NotNil(t, g.allowedPeers)
}

func TestGater_BlockPeer(t *testing.T) {
	g := New()

	// 初始时应该允许
	assert.True(t, g.InterceptPeerDial("peer1"))

	// 加入黑名单
	g.BlockPeer("peer1")
	assert.False(t, g.InterceptPeerDial("peer1"))

	// 其他节点不受影响
	assert.True(t, g.InterceptPeerDial("peer2"))
}

func TestGater_UnblockPeer(t *testing.T) {
	g := New()

	g.BlockPeer("peer1")
	assert.False(t, g.InterceptPeerDial("peer1"))

	// 解除黑名单
	g.UnblockPeer("peer1")
	assert.True(t, g.InterceptPeerDial("peer1"))
}

func TestGater_BlockAddr(t *testing.T) {
	g := New()

	addr := "/ip4/192.168.1.1/tcp/4001"

	// 初始时应该允许
	assert.True(t, g.InterceptAccept(addr))
	assert.True(t, g.InterceptAddrDial("peer1", addr))

	// 加入黑名单
	g.BlockAddr(addr)
	assert.False(t, g.InterceptAccept(addr))
	assert.False(t, g.InterceptAddrDial("peer1", addr))

	// 其他地址不受影响
	assert.True(t, g.InterceptAccept("/ip4/10.0.0.1/tcp/4001"))
}

func TestGater_UnblockAddr(t *testing.T) {
	g := New()

	addr := "/ip4/192.168.1.1/tcp/4001"

	g.BlockAddr(addr)
	assert.False(t, g.InterceptAccept(addr))

	// 解除黑名单
	g.UnblockAddr(addr)
	assert.True(t, g.InterceptAccept(addr))
}

func TestGater_InterceptSecured(t *testing.T) {
	g := New()

	// 初始时应该允许
	assert.True(t, g.InterceptSecured(0, "peer1"))
	assert.True(t, g.InterceptSecured(1, "peer1"))

	// 加入黑名单
	g.BlockPeer("peer1")
	assert.False(t, g.InterceptSecured(0, "peer1"))
	assert.False(t, g.InterceptSecured(1, "peer1"))
}

func TestGater_InterceptAddrDial_PeerBlocked(t *testing.T) {
	g := New()

	// 节点被阻止时，地址拨号也应该被阻止
	g.BlockPeer("peer1")
	assert.False(t, g.InterceptAddrDial("peer1", "/ip4/10.0.0.1/tcp/4001"))
}

func TestGater_AllowPeer(t *testing.T) {
	g := New()

	g.AllowPeer("peer1")

	// 验证白名单存在
	g.mu.RLock()
	_, exists := g.allowedPeers["peer1"]
	g.mu.RUnlock()

	assert.True(t, exists)
}

func TestGater_DisallowPeer(t *testing.T) {
	g := New()

	g.AllowPeer("peer1")
	g.DisallowPeer("peer1")

	// 验证已从白名单移除
	g.mu.RLock()
	_, exists := g.allowedPeers["peer1"]
	g.mu.RUnlock()

	assert.False(t, exists)
}

func TestGater_Concurrent(t *testing.T) {
	g := New()

	var wg sync.WaitGroup

	// 并发操作
	for i := 0; i < 100; i++ {
		wg.Add(4)
		go func(idx int) {
			defer wg.Done()
			peerID := string(rune('A' + idx%26))
			g.BlockPeer(peerID)
		}(i)
		go func(idx int) {
			defer wg.Done()
			peerID := string(rune('A' + idx%26))
			g.UnblockPeer(peerID)
		}(i)
		go func(idx int) {
			defer wg.Done()
			peerID := string(rune('A' + idx%26))
			g.InterceptPeerDial(peerID)
		}(i)
		go func(idx int) {
			defer wg.Done()
			addr := "/ip4/10.0.0.1/tcp/4001"
			g.InterceptAccept(addr)
		}(i)
	}

	wg.Wait()
}

// ============================================================================
//                              Filter 测试
// ============================================================================

func TestFilter_New(t *testing.T) {
	f := NewFilter(true)
	require.NotNil(t, f)
	assert.True(t, f.defaultAllow)

	f2 := NewFilter(false)
	assert.False(t, f2.defaultAllow)
}

func TestFilter_AllowCIDR(t *testing.T) {
	f := NewFilter(false)

	err := f.AllowCIDR("192.168.1.0/24")
	require.NoError(t, err)

	// 范围内的 IP 应该允许
	assert.True(t, f.AllowIP(net.ParseIP("192.168.1.100")))

	// 范围外的 IP 应该拒绝（defaultAllow=false）
	assert.False(t, f.AllowIP(net.ParseIP("10.0.0.1")))
}

func TestFilter_AllowCIDR_Invalid(t *testing.T) {
	f := NewFilter(true)

	err := f.AllowCIDR("invalid-cidr")
	require.Error(t, err)
}

func TestFilter_BlockCIDR(t *testing.T) {
	f := NewFilter(true)

	err := f.BlockCIDR("10.0.0.0/8")
	require.NoError(t, err)

	// 被阻止的范围
	assert.False(t, f.AllowIP(net.ParseIP("10.1.2.3")))

	// 其他 IP 应该允许（defaultAllow=true）
	assert.True(t, f.AllowIP(net.ParseIP("192.168.1.1")))
}

func TestFilter_BlockCIDR_Invalid(t *testing.T) {
	f := NewFilter(true)

	err := f.BlockCIDR("invalid-cidr")
	require.Error(t, err)
}

func TestFilter_AllowIP_BlockedTakesPrecedence(t *testing.T) {
	f := NewFilter(true)

	// 同时添加到允许和阻止列表
	f.AllowCIDR("192.168.0.0/16")
	f.BlockCIDR("192.168.1.0/24")

	// 被阻止的子网应该拒绝
	assert.False(t, f.AllowIP(net.ParseIP("192.168.1.100")))

	// 允许的其他子网应该允许
	assert.True(t, f.AllowIP(net.ParseIP("192.168.2.100")))
}

func TestFilter_AllowAddr(t *testing.T) {
	f := NewFilter(true)

	f.BlockCIDR("10.0.0.0/8")

	// 测试带端口的地址
	assert.False(t, f.AllowAddr("10.1.2.3:8080"))
	assert.True(t, f.AllowAddr("192.168.1.1:8080"))

	// 测试不带端口的地址
	assert.False(t, f.AllowAddr("10.1.2.3"))
	assert.True(t, f.AllowAddr("192.168.1.1"))
}

func TestFilter_AllowAddr_InvalidIP(t *testing.T) {
	f := NewFilter(true)

	// 无法解析的地址返回默认值
	assert.True(t, f.AllowAddr("invalid-host"))

	f2 := NewFilter(false)
	assert.False(t, f2.AllowAddr("invalid-host"))
}

func TestFilter_Reset(t *testing.T) {
	f := NewFilter(true)

	f.AllowCIDR("192.168.0.0/16")
	f.BlockCIDR("10.0.0.0/8")

	f.Reset()

	assert.Nil(t, f.allowedCIDRs)
	assert.Nil(t, f.blockedCIDRs)

	// 重置后应该回到默认行为
	assert.True(t, f.AllowIP(net.ParseIP("10.1.2.3")))
}

func TestFilter_Concurrent(t *testing.T) {
	f := NewFilter(true)

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			f.AllowCIDR("192.168.1.0/24")
		}()
		go func() {
			defer wg.Done()
			f.BlockCIDR("10.0.0.0/8")
		}()
		go func() {
			defer wg.Done()
			f.AllowIP(net.ParseIP("192.168.1.1"))
		}()
	}

	wg.Wait()
}

func TestFilter_AllowIP_EmptyAllowedList(t *testing.T) {
	f := NewFilter(true)

	// 没有允许列表时，使用默认值
	assert.True(t, f.AllowIP(net.ParseIP("192.168.1.1")))

	f2 := NewFilter(false)
	assert.False(t, f2.AllowIP(net.ParseIP("192.168.1.1")))
}

func TestFilter_AllowIP_WithAllowedList(t *testing.T) {
	f := NewFilter(false)

	f.AllowCIDR("192.168.1.0/24")

	// 在允许列表中
	assert.True(t, f.AllowIP(net.ParseIP("192.168.1.100")))

	// 不在允许列表中（有允许列表时，不在列表中的默认拒绝）
	assert.False(t, f.AllowIP(net.ParseIP("10.0.0.1")))
}
