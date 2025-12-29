package connmgr

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              内存存储实现
// ============================================================================

// MemoryGaterStore 内存存储实现
//
// 不持久化，仅用于测试或临时使用
type MemoryGaterStore struct {
	peers   []types.NodeID
	addrs   []net.IP
	subnets []*net.IPNet
	mu      sync.RWMutex
}

// 确保实现接口
var _ connmgr.GaterStore = (*MemoryGaterStore)(nil)

// NewMemoryGaterStore 创建内存存储
func NewMemoryGaterStore() *MemoryGaterStore {
	return &MemoryGaterStore{
		peers:   make([]types.NodeID, 0),
		addrs:   make([]net.IP, 0),
		subnets: make([]*net.IPNet, 0),
	}
}

// SavePeer 保存被阻止的节点
func (s *MemoryGaterStore) SavePeer(nodeID types.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已存在
	for _, p := range s.peers {
		if p == nodeID {
			return nil
		}
	}
	s.peers = append(s.peers, nodeID)
	return nil
}

// DeletePeer 删除被阻止的节点
func (s *MemoryGaterStore) DeletePeer(nodeID types.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, p := range s.peers {
		if p == nodeID {
			s.peers = append(s.peers[:i], s.peers[i+1:]...)
			return nil
		}
	}
	return nil
}

// LoadPeers 加载所有被阻止的节点
func (s *MemoryGaterStore) LoadPeers() ([]types.NodeID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.NodeID, len(s.peers))
	copy(result, s.peers)
	return result, nil
}

// SaveAddr 保存被阻止的 IP 地址
func (s *MemoryGaterStore) SaveAddr(ip net.IP) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已存在
	for _, a := range s.addrs {
		if a.Equal(ip) {
			return nil
		}
	}
	s.addrs = append(s.addrs, ip)
	return nil
}

// DeleteAddr 删除被阻止的 IP 地址
func (s *MemoryGaterStore) DeleteAddr(ip net.IP) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, a := range s.addrs {
		if a.Equal(ip) {
			s.addrs = append(s.addrs[:i], s.addrs[i+1:]...)
			return nil
		}
	}
	return nil
}

// LoadAddrs 加载所有被阻止的 IP 地址
func (s *MemoryGaterStore) LoadAddrs() ([]net.IP, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]net.IP, len(s.addrs))
	copy(result, s.addrs)
	return result, nil
}

// SaveSubnet 保存被阻止的子网
func (s *MemoryGaterStore) SaveSubnet(ipnet *net.IPNet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已存在
	for _, subnet := range s.subnets {
		if subnet.String() == ipnet.String() {
			return nil
		}
	}
	s.subnets = append(s.subnets, ipnet)
	return nil
}

// DeleteSubnet 删除被阻止的子网
func (s *MemoryGaterStore) DeleteSubnet(ipnet *net.IPNet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, subnet := range s.subnets {
		if subnet.String() == ipnet.String() {
			s.subnets = append(s.subnets[:i], s.subnets[i+1:]...)
			return nil
		}
	}
	return nil
}

// LoadSubnets 加载所有被阻止的子网
func (s *MemoryGaterStore) LoadSubnets() ([]*net.IPNet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*net.IPNet, len(s.subnets))
	copy(result, s.subnets)
	return result, nil
}

// ============================================================================
//                              文件存储实现
// ============================================================================

// FileGaterStore 文件存储实现
//
// 将黑名单规则持久化到 JSON 文件
type FileGaterStore struct {
	path string
	data *gaterData
	mu   sync.RWMutex
}

// gaterData 存储数据结构
type gaterData struct {
	Peers   []string `json:"peers"`
	Addrs   []string `json:"addrs"`
	Subnets []string `json:"subnets"`
}

// 确保实现接口
var _ connmgr.GaterStore = (*FileGaterStore)(nil)

// NewFileGaterStore 创建文件存储
func NewFileGaterStore(path string) (*FileGaterStore, error) {
	s := &FileGaterStore{
		path: path,
		data: &gaterData{
			Peers:   make([]string, 0),
			Addrs:   make([]string, 0),
			Subnets: make([]string, 0),
		},
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}

	// 尝试加载现有文件
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return s, nil
}

// load 从文件加载数据
func (s *FileGaterStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, s.data)
}

// save 保存数据到文件（原子写入）
func (s *FileGaterStore) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	// 原子写入：先写入临时文件，再重命名
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	// 原子重命名（同一文件系统上是原子操作）
	return os.Rename(tmpPath, s.path)
}

// SavePeer 保存被阻止的节点
func (s *FileGaterStore) SavePeer(nodeID types.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	peerStr := nodeID.String()

	// 检查是否已存在
	for _, p := range s.data.Peers {
		if p == peerStr {
			return nil
		}
	}

	s.data.Peers = append(s.data.Peers, peerStr)
	return s.save()
}

// DeletePeer 删除被阻止的节点
func (s *FileGaterStore) DeletePeer(nodeID types.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	peerStr := nodeID.String()

	for i, p := range s.data.Peers {
		if p == peerStr {
			s.data.Peers = append(s.data.Peers[:i], s.data.Peers[i+1:]...)
			return s.save()
		}
	}
	return nil
}

// LoadPeers 加载所有被阻止的节点
func (s *FileGaterStore) LoadPeers() ([]types.NodeID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.NodeID, 0, len(s.data.Peers))
	var invalidCount int
	for _, peerStr := range s.data.Peers {
		nodeID, err := types.ParseNodeID(peerStr)
		if err != nil {
			invalidCount++
			continue // 跳过无效的
		}
		result = append(result, nodeID)
	}
	// 如果有无效条目，返回非致命错误信息（但仍返回有效数据）
	if invalidCount > 0 {
		return result, fmt.Errorf("跳过 %d 个无效的 peer 条目", invalidCount)
	}
	return result, nil
}

// SaveAddr 保存被阻止的 IP 地址
func (s *FileGaterStore) SaveAddr(ip net.IP) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ipStr := ip.String()

	// 检查是否已存在
	for _, a := range s.data.Addrs {
		if a == ipStr {
			return nil
		}
	}

	s.data.Addrs = append(s.data.Addrs, ipStr)
	return s.save()
}

// DeleteAddr 删除被阻止的 IP 地址
func (s *FileGaterStore) DeleteAddr(ip net.IP) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ipStr := ip.String()

	for i, a := range s.data.Addrs {
		if a == ipStr {
			s.data.Addrs = append(s.data.Addrs[:i], s.data.Addrs[i+1:]...)
			return s.save()
		}
	}
	return nil
}

// LoadAddrs 加载所有被阻止的 IP 地址
func (s *FileGaterStore) LoadAddrs() ([]net.IP, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]net.IP, 0, len(s.data.Addrs))
	for _, ipStr := range s.data.Addrs {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			result = append(result, ip)
		}
	}
	return result, nil
}

// SaveSubnet 保存被阻止的子网
func (s *FileGaterStore) SaveSubnet(ipnet *net.IPNet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	subnetStr := ipnet.String()

	// 检查是否已存在
	for _, subnet := range s.data.Subnets {
		if subnet == subnetStr {
			return nil
		}
	}

	s.data.Subnets = append(s.data.Subnets, subnetStr)
	return s.save()
}

// DeleteSubnet 删除被阻止的子网
func (s *FileGaterStore) DeleteSubnet(ipnet *net.IPNet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	subnetStr := ipnet.String()

	for i, subnet := range s.data.Subnets {
		if subnet == subnetStr {
			s.data.Subnets = append(s.data.Subnets[:i], s.data.Subnets[i+1:]...)
			return s.save()
		}
	}
	return nil
}

// LoadSubnets 加载所有被阻止的子网
func (s *FileGaterStore) LoadSubnets() ([]*net.IPNet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*net.IPNet, 0, len(s.data.Subnets))
	for _, subnetStr := range s.data.Subnets {
		_, ipnet, err := net.ParseCIDR(subnetStr)
		if err == nil && ipnet != nil {
			result = append(result, ipnet)
		}
	}
	return result, nil
}

