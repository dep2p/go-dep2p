package connmgr

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              ConnectionGater 测试
// ============================================================================

func TestConnectionGater_BlockPeer(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

	// 阻止节点
	if err := gater.BlockPeer(nodeID); err != nil {
		t.Fatalf("BlockPeer 失败: %v", err)
	}

	// 检查是否被阻止
	if !gater.IsBlocked(nodeID) {
		t.Error("节点应该被阻止")
	}

	// 列表应该包含该节点
	peers := gater.ListBlockedPeers()
	if len(peers) != 1 {
		t.Errorf("ListBlockedPeers 返回 %d 个，期望 1", len(peers))
	}

	// 解除阻止
	if err := gater.UnblockPeer(nodeID); err != nil {
		t.Fatalf("UnblockPeer 失败: %v", err)
	}

	if gater.IsBlocked(nodeID) {
		t.Error("节点不应该再被阻止")
	}
}

func TestConnectionGater_BlockAddr(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	ip := net.ParseIP("192.168.1.100")

	// 阻止 IP
	if err := gater.BlockAddr(ip); err != nil {
		t.Fatalf("BlockAddr 失败: %v", err)
	}

	// 检查是否被阻止
	if !gater.IsAddrBlocked(ip) {
		t.Error("IP 应该被阻止")
	}

	// 列表应该包含该 IP
	addrs := gater.ListBlockedAddrs()
	if len(addrs) != 1 {
		t.Errorf("ListBlockedAddrs 返回 %d 个，期望 1", len(addrs))
	}

	// 解除阻止
	if err := gater.UnblockAddr(ip); err != nil {
		t.Fatalf("UnblockAddr 失败: %v", err)
	}

	if gater.IsAddrBlocked(ip) {
		t.Error("IP 不应该再被阻止")
	}
}

func TestConnectionGater_BlockSubnet(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	_, subnet, _ := net.ParseCIDR("192.168.1.0/24")

	// 阻止子网
	if err := gater.BlockSubnet(subnet); err != nil {
		t.Fatalf("BlockSubnet 失败: %v", err)
	}

	// 子网内的 IP 应该被阻止
	ip := net.ParseIP("192.168.1.100")
	if !gater.IsAddrBlocked(ip) {
		t.Error("子网内的 IP 应该被阻止")
	}

	// 子网外的 IP 不应该被阻止
	ip2 := net.ParseIP("10.0.0.1")
	if gater.IsAddrBlocked(ip2) {
		t.Error("子网外的 IP 不应该被阻止")
	}

	// 列表应该包含该子网
	subnets := gater.ListBlockedSubnets()
	if len(subnets) != 1 {
		t.Errorf("ListBlockedSubnets 返回 %d 个，期望 1", len(subnets))
	}

	// 解除阻止
	if err := gater.UnblockSubnet(subnet); err != nil {
		t.Fatalf("UnblockSubnet 失败: %v", err)
	}

	if gater.IsAddrBlocked(ip) {
		t.Error("子网内的 IP 不应该再被阻止")
	}
}

func TestConnectionGater_InterceptPeerDial(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

	// 未阻止时应该允许
	if !gater.InterceptPeerDial(nodeID) {
		t.Error("未阻止的节点应该允许拨号")
	}

	// 阻止后应该拒绝
	gater.BlockPeer(nodeID)
	if gater.InterceptPeerDial(nodeID) {
		t.Error("被阻止的节点应该拒绝拨号")
	}
}

func TestConnectionGater_InterceptAccept(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	ip := net.ParseIP("192.168.1.100")

	// 未阻止时应该允许
	if !gater.InterceptAccept("192.168.1.100:8080") {
		t.Error("未阻止的 IP 应该允许接受连接")
	}

	// 阻止后应该拒绝
	gater.BlockAddr(ip)
	if gater.InterceptAccept("192.168.1.100:8080") {
		t.Error("被阻止的 IP 应该拒绝接受连接")
	}

	// Relay 地址应该允许（不是传统 IP:Port 格式）
	relayAddr := "/p2p/QmRelayID/p2p-circuit/p2p/QmRemoteID"
	if !gater.InterceptAccept(relayAddr) {
		t.Error("Relay 地址应该允许接受连接")
	}

	// Relay multiaddr（包含 /ip4/.../p2p/.../p2p-circuit/...）也应允许
	relayMA := "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelayID/p2p-circuit/p2p/QmRemoteID"
	if !gater.InterceptAccept(relayMA) {
		t.Error("Relay multiaddr 应该允许接受连接")
	}

	// multiaddr 可提取 IP 时，仍应执行黑名单（覆盖：未来 transport RemoteAddr 返回 multiaddr）
	gater.BlockAddr(ip)
	if gater.InterceptAccept("/ip4/192.168.1.100/tcp/4001") {
		t.Error("被阻止的 IP（multiaddr 形式）应该拒绝接受连接")
	}
}

func TestConnectionGater_InterceptSecured(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

	// 入站连接检查
	if !gater.InterceptSecured(types.DirInbound, nodeID) {
		t.Error("未阻止的节点应该允许入站连接")
	}

	gater.BlockPeer(nodeID)
	if gater.InterceptSecured(types.DirInbound, nodeID) {
		t.Error("被阻止的节点应该拒绝入站连接")
	}

	// 出站连接应该始终通过（已在 InterceptPeerDial 检查）
	if !gater.InterceptSecured(types.DirOutbound, nodeID) {
		t.Error("出站连接应该始终通过 InterceptSecured")
	}
}

func TestConnectionGater_Clear(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	// 添加一些规则
	gater.BlockPeer(types.NodeID{1, 2, 3, 4, 5, 6, 7, 8})
	gater.BlockAddr(net.ParseIP("192.168.1.100"))
	_, subnet, _ := net.ParseCIDR("10.0.0.0/8")
	gater.BlockSubnet(subnet)

	// 清除
	gater.Clear()

	// 验证清除
	stats := gater.Stats()
	if stats.BlockedPeers != 0 {
		t.Errorf("BlockedPeers = %d, 期望 0", stats.BlockedPeers)
	}
	if stats.BlockedAddrs != 0 {
		t.Errorf("BlockedAddrs = %d, 期望 0", stats.BlockedAddrs)
	}
	if stats.BlockedSubnets != 0 {
		t.Errorf("BlockedSubnets = %d, 期望 0", stats.BlockedSubnets)
	}
}

func TestConnectionGater_Stats(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	// 添加规则
	gater.BlockPeer(types.NodeID{1, 2, 3, 4, 5, 6, 7, 8})
	gater.BlockPeer(types.NodeID{2, 3, 4, 5, 6, 7, 8, 9})
	gater.BlockAddr(net.ParseIP("192.168.1.100"))

	stats := gater.Stats()
	if stats.BlockedPeers != 2 {
		t.Errorf("BlockedPeers = %d, 期望 2", stats.BlockedPeers)
	}
	if stats.BlockedAddrs != 1 {
		t.Errorf("BlockedAddrs = %d, 期望 1", stats.BlockedAddrs)
	}
}

func TestConnectionGater_Disabled(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	config.Enabled = false
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	gater.BlockPeer(nodeID)

	// 禁用时应该始终允许
	if !gater.InterceptPeerDial(nodeID) {
		t.Error("禁用时应该允许拨号")
	}
	if !gater.InterceptAccept("192.168.1.100:8080") {
		t.Error("禁用时应该允许接受连接")
	}
}

// ============================================================================
//                              MemoryGaterStore 测试
// ============================================================================

func TestMemoryGaterStore_Peers(t *testing.T) {
	store := NewMemoryGaterStore()

	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

	// 保存
	if err := store.SavePeer(nodeID); err != nil {
		t.Fatalf("SavePeer 失败: %v", err)
	}

	// 加载
	peers, err := store.LoadPeers()
	if err != nil {
		t.Fatalf("LoadPeers 失败: %v", err)
	}
	if len(peers) != 1 {
		t.Errorf("LoadPeers 返回 %d 个，期望 1", len(peers))
	}

	// 删除
	if err := store.DeletePeer(nodeID); err != nil {
		t.Fatalf("DeletePeer 失败: %v", err)
	}

	peers, _ = store.LoadPeers()
	if len(peers) != 0 {
		t.Errorf("删除后 LoadPeers 返回 %d 个，期望 0", len(peers))
	}
}

func TestMemoryGaterStore_Addrs(t *testing.T) {
	store := NewMemoryGaterStore()

	ip := net.ParseIP("192.168.1.100")

	// 保存
	if err := store.SaveAddr(ip); err != nil {
		t.Fatalf("SaveAddr 失败: %v", err)
	}

	// 加载
	addrs, err := store.LoadAddrs()
	if err != nil {
		t.Fatalf("LoadAddrs 失败: %v", err)
	}
	if len(addrs) != 1 {
		t.Errorf("LoadAddrs 返回 %d 个，期望 1", len(addrs))
	}

	// 删除
	if err := store.DeleteAddr(ip); err != nil {
		t.Fatalf("DeleteAddr 失败: %v", err)
	}

	addrs, _ = store.LoadAddrs()
	if len(addrs) != 0 {
		t.Errorf("删除后 LoadAddrs 返回 %d 个，期望 0", len(addrs))
	}
}

func TestMemoryGaterStore_Subnets(t *testing.T) {
	store := NewMemoryGaterStore()

	_, subnet, _ := net.ParseCIDR("192.168.1.0/24")

	// 保存
	if err := store.SaveSubnet(subnet); err != nil {
		t.Fatalf("SaveSubnet 失败: %v", err)
	}

	// 加载
	subnets, err := store.LoadSubnets()
	if err != nil {
		t.Fatalf("LoadSubnets 失败: %v", err)
	}
	if len(subnets) != 1 {
		t.Errorf("LoadSubnets 返回 %d 个，期望 1", len(subnets))
	}

	// 删除
	if err := store.DeleteSubnet(subnet); err != nil {
		t.Fatalf("DeleteSubnet 失败: %v", err)
	}

	subnets, _ = store.LoadSubnets()
	if len(subnets) != 0 {
		t.Errorf("删除后 LoadSubnets 返回 %d 个，期望 0", len(subnets))
	}
}

// ============================================================================
//                              FileGaterStore 测试
// ============================================================================

func TestFileGaterStore_Persistence(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "gater_test")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "blocklist.json")

	// 创建存储并添加规则
	store, err := NewFileGaterStore(path)
	if err != nil {
		t.Fatalf("创建 FileGaterStore 失败: %v", err)
	}

	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	if err := store.SavePeer(nodeID); err != nil {
		t.Fatalf("SavePeer 失败: %v", err)
	}

	ip := net.ParseIP("192.168.1.100")
	if err := store.SaveAddr(ip); err != nil {
		t.Fatalf("SaveAddr 失败: %v", err)
	}

	// 重新加载
	store2, err := NewFileGaterStore(path)
	if err != nil {
		t.Fatalf("重新创建 FileGaterStore 失败: %v", err)
	}

	peers, _ := store2.LoadPeers()
	if len(peers) != 1 {
		t.Errorf("LoadPeers 返回 %d 个，期望 1", len(peers))
	}

	addrs, _ := store2.LoadAddrs()
	if len(addrs) != 1 {
		t.Errorf("LoadAddrs 返回 %d 个，期望 1", len(addrs))
	}
}

// ============================================================================
//                              集成测试
// ============================================================================

func TestConnectionGater_WithStore(t *testing.T) {
	store := NewMemoryGaterStore()

	config := connmgr.DefaultGaterConfig()
	config.Store = store

	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

	// 阻止节点
	if err := gater.BlockPeer(nodeID); err != nil {
		t.Fatalf("BlockPeer 失败: %v", err)
	}

	// 验证存储
	peers, _ := store.LoadPeers()
	if len(peers) != 1 {
		t.Errorf("存储中应该有 1 个节点，实际 %d", len(peers))
	}

	// 解除阻止
	if err := gater.UnblockPeer(nodeID); err != nil {
		t.Fatalf("UnblockPeer 失败: %v", err)
	}

	// 验证存储
	peers, _ = store.LoadPeers()
	if len(peers) != 0 {
		t.Errorf("存储中应该有 0 个节点，实际 %d", len(peers))
	}
}

func TestConnectionGater_ExportRules(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	// 添加规则
	gater.BlockPeer(types.NodeID{1, 2, 3, 4, 5, 6, 7, 8})
	gater.BlockAddr(net.ParseIP("192.168.1.100"))
	_, subnet, _ := net.ParseCIDR("10.0.0.0/8")
	gater.BlockSubnet(subnet)

	// 导出
	peers, addrs, subnets := gater.ExportRules()

	if len(peers) != 1 {
		t.Errorf("导出 peers = %d, 期望 1", len(peers))
	}
	if len(addrs) != 1 {
		t.Errorf("导出 addrs = %d, 期望 1", len(addrs))
	}
	if len(subnets) != 1 {
		t.Errorf("导出 subnets = %d, 期望 1", len(subnets))
	}
}

func TestConnectionGater_ImportPeers(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	peers := []types.NodeID{
		{1, 2, 3, 4, 5, 6, 7, 8},
		{2, 3, 4, 5, 6, 7, 8, 9},
		{3, 4, 5, 6, 7, 8, 9, 10},
	}

	if err := gater.ImportPeers(peers); err != nil {
		t.Fatalf("ImportPeers 失败: %v", err)
	}

	stats := gater.Stats()
	if stats.BlockedPeers != 3 {
		t.Errorf("BlockedPeers = %d, 期望 3", stats.BlockedPeers)
	}
}

// ============================================================================
//                              新增测试 - 审查修复验证
// ============================================================================

// TestInterceptAccept_InvalidAddress 测试 InterceptAccept 对无效地址的处理
func TestInterceptAccept_InvalidAddress(t *testing.T) {
	config := connmgr.DefaultGaterConfig()
	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	// 测试无效地址格式应该被拒绝
	invalidAddrs := []string{
		"not-an-ip",
		":::invalid",
		"",
		"abc.def.ghi.jkl",
	}

	for _, addr := range invalidAddrs {
		result := gater.InterceptAccept(addr)
		if result {
			t.Errorf("InterceptAccept(%q) = true, 期望 false（无效地址应被拒绝）", addr)
		}
	}

	// 有效地址应该放行
	validAddrs := []string{
		"192.168.1.1:8080",
		"10.0.0.1:443",
		"[::1]:8080",
	}

	for _, addr := range validAddrs {
		result := gater.InterceptAccept(addr)
		if !result {
			t.Errorf("InterceptAccept(%q) = false, 期望 true（有效地址应放行）", addr)
		}
	}
}

// TestFileGaterStore_AtomicWrite 测试文件存储的原子写入
func TestFileGaterStore_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test_blocklist.json")

	store, err := NewFileGaterStore(storePath)
	if err != nil {
		t.Fatalf("创建 FileGaterStore 失败: %v", err)
	}

	// 添加一些数据
	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	if err := store.SavePeer(nodeID); err != nil {
		t.Fatalf("SavePeer 失败: %v", err)
	}

	// 确保没有残留的临时文件
	tmpPath := storePath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("临时文件应该被清理: ", tmpPath)
	}

	// 确保主文件存在且有效
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		t.Error("存储文件应该存在")
	}

	// 验证数据可以正确读取
	peers, err := store.LoadPeers()
	if err != nil {
		t.Fatalf("LoadPeers 失败: %v", err)
	}
	if len(peers) != 1 || peers[0] != nodeID {
		t.Errorf("LoadPeers 返回 %v, 期望 [%v]", peers, nodeID)
	}
}

// TestBlockPeer_PersistenceFirst 测试 BlockPeer 先持久化再更新内存
func TestBlockPeer_PersistenceFirst(t *testing.T) {
	// 创建一个会失败的 store
	failingStore := &failingGaterStore{shouldFail: true}

	config := connmgr.GaterConfig{
		Enabled: true,
		Store:   failingStore,
	}

	gater, err := NewConnectionGater(config)
	if err != nil {
		t.Fatalf("创建 Gater 失败: %v", err)
	}

	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

	// BlockPeer 应该失败
	err = gater.BlockPeer(nodeID)
	if err == nil {
		t.Error("BlockPeer 应该因持久化失败而返回错误")
	}

	// 内存中不应该有这个节点（因为持久化失败了）
	if gater.IsBlocked(nodeID) {
		t.Error("持久化失败时，节点不应该在内存黑名单中")
	}
}

// failingGaterStore 模拟持久化失败的 store
type failingGaterStore struct {
	shouldFail bool
}

func (s *failingGaterStore) SavePeer(nodeID types.NodeID) error {
	if s.shouldFail {
		return os.ErrPermission
	}
	return nil
}

func (s *failingGaterStore) DeletePeer(nodeID types.NodeID) error {
	return nil
}

func (s *failingGaterStore) LoadPeers() ([]types.NodeID, error) {
	return nil, nil
}

func (s *failingGaterStore) SaveAddr(ip net.IP) error {
	return nil
}

func (s *failingGaterStore) DeleteAddr(ip net.IP) error {
	return nil
}

func (s *failingGaterStore) LoadAddrs() ([]net.IP, error) {
	return nil, nil
}

func (s *failingGaterStore) SaveSubnet(ipnet *net.IPNet) error {
	return nil
}

func (s *failingGaterStore) DeleteSubnet(ipnet *net.IPNet) error {
	return nil
}

func (s *failingGaterStore) LoadSubnets() ([]*net.IPNet, error) {
	return nil, nil
}

