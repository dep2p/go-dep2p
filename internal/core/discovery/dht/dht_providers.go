// Package dht 提供分布式哈希表实现
package dht

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              类型定义
// ============================================================================

// DefaultProviderTTL 默认 Provider 记录 TTL
const DefaultProviderTTL = 24 * time.Hour

// ProviderInfo provider 信息（带 TTL/时间戳）
// 用于 Network 接口在 GET_PROVIDERS 中返回 TTL 元数据。
type ProviderInfo struct {
	ID        types.NodeID
	Addrs     []string
	Timestamp time.Time
	TTL       time.Duration
}

// providerEntry Provider 条目
type providerEntry struct {
	ID        types.NodeID
	Addrs     []string
	Timestamp time.Time
	TTL       time.Duration
}

// ============================================================================
//                              Provider 本地存储
// ============================================================================

// addProviderLocal 添加 provider 到本地存储
func (d *DHT) addProviderLocal(key string, id types.NodeID, addrs []string, ttl time.Duration) {
	d.providersMu.Lock()
	defer d.providersMu.Unlock()

	// 检查是否已存在
	entries := d.providers[key]
	for i, entry := range entries {
		if entry.ID == id {
			// 更新已存在的条目
			entries[i].Addrs = addrs
			entries[i].Timestamp = time.Now()
			if ttl > 0 {
				entries[i].TTL = ttl
			}
			return
		}
	}

	// 添加新条目
	d.providers[key] = append(entries, providerEntry{
		ID:        id,
		Addrs:     addrs,
		Timestamp: time.Now(),
		TTL:       ttl,
	})
}

func (d *DHT) addProviderLocalWithMeta(key string, id types.NodeID, addrs []string, timestamp time.Time, ttl time.Duration) {
	d.providersMu.Lock()
	defer d.providersMu.Unlock()

	entries := d.providers[key]
	for i, entry := range entries {
		if entry.ID == id {
			entries[i].Addrs = addrs
			if !timestamp.IsZero() {
				entries[i].Timestamp = timestamp
			} else {
				entries[i].Timestamp = time.Now()
			}
			if ttl > 0 {
				entries[i].TTL = ttl
			}
			return
		}
	}

	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	d.providers[key] = append(entries, providerEntry{
		ID:        id,
		Addrs:     addrs,
		Timestamp: timestamp,
		TTL:       ttl,
	})
}

// getProvidersLocal 从本地存储获取 providers
func (d *DHT) getProvidersLocal(key string) []providerEntry {
	d.providersMu.RLock()
	defer d.providersMu.RUnlock()

	entries := d.providers[key]
	if len(entries) == 0 {
		return nil
	}

	// 过滤过期的 providers（按条目 TTL，TTL<=0 视为 DefaultProviderTTL）
	result := make([]providerEntry, 0, len(entries))
	for _, entry := range entries {
		ttl := entry.TTL
		if ttl <= 0 {
			ttl = DefaultProviderTTL
		}
		if time.Since(entry.Timestamp) < ttl {
			result = append(result, entry)
		}
	}

	return result
}

// cleanupProviders 清理过期的 providers
func (d *DHT) cleanupProviders() {
	d.providersMu.Lock()
	defer d.providersMu.Unlock()

	for key, entries := range d.providers {
		valid := make([]providerEntry, 0, len(entries))
		for _, entry := range entries {
			ttl := entry.TTL
			if ttl <= 0 {
				ttl = DefaultProviderTTL
			}
			if time.Since(entry.Timestamp) < ttl {
				valid = append(valid, entry)
			}
		}
		if len(valid) == 0 {
			delete(d.providers, key)
		} else {
			d.providers[key] = valid
		}
	}
}

// removeProviderLocal 从本地存储移除 provider
func (d *DHT) removeProviderLocal(key string, id types.NodeID) {
	d.providersMu.Lock()
	defer d.providersMu.Unlock()

	entries := d.providers[key]
	if len(entries) == 0 {
		return
	}

	// 过滤掉指定 ID 的 provider
	newEntries := make([]providerEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.ID != id {
			newEntries = append(newEntries, entry)
		}
	}

	if len(newEntries) == 0 {
		delete(d.providers, key)
	} else {
		d.providers[key] = newEntries
	}
}

// ============================================================================
//                              Provider 公开 API
// ============================================================================

// AddProvider 注册本节点为指定 namespace 的 provider
//
// 将本节点注册到 DHT 作为 namespace 的 provider。
// Key 格式：dep2p/v1/sys/<namespace>
//
// T2 修复：存储和距离计算使用 SHA256(key)，网络消息传输原始 key。
func (d *DHT) AddProvider(ctx context.Context, namespace string) error {
	return d.addProviderWithTTL(ctx, namespace, DefaultProviderTTL)
}

func (d *DHT) addProviderWithTTL(ctx context.Context, namespace string, ttl time.Duration) error {
	if atomic.LoadInt32(&d.running) == 0 {
		return ErrDHTClosed
	}

	if namespace == "" {
		return errors.New("empty namespace")
	}
	if ttl <= 0 {
		ttl = DefaultProviderTTL
	}
	// Provider TTL 上限：避免过长 TTL 导致撤销/变更不及时（与 Layer1 约束对齐）
	if ttl > DefaultProviderTTL {
		ttl = DefaultProviderTTL
	}

	// Layer1 修复：使用 NormalizeNamespace 防止双前缀，并根据 scope/realmID 构建 key
	parsed := NormalizeNamespace(namespace)
	if parsed.Namespace != namespace && namespace != "" {
		log.Warn("namespace 已归一化（防止双前缀）",
			"original", namespace,
			"normalized", parsed.Namespace,
			"scope", parsed.Scope.String())
	}

	// 根据 scope 决定 key：sys 域使用 sys 前缀，realm 域使用 realm 前缀
	// 当前 DHT AddProvider 主要用于系统服务（如 relay），强制 sys 域
	key := BuildProviderKeyWithParsed(parsed, string(d.realmID))

	// T2: 使用哈希后的 key 作为存储 key
	hashedKey := HashKeyString(key)

	// 本地存储
	d.storeMu.RLock()
	addrs := make([]string, len(d.localAddrs))
	copy(addrs, d.localAddrs)
	d.storeMu.RUnlock()

	d.addProviderLocal(hashedKey, d.localID, addrs, ttl)

	// 发布到 DHT 网络
	d.networkMu.RLock()
	network := d.network
	d.networkMu.RUnlock()

	if network == nil {
		log.Debug("网络层未就绪，仅本地存储 provider", "key", key)
		return nil
	}

	// T2: 距离计算使用哈希后的 key bytes
	keyBytes := HashKey(key)
	closestPeers := d.routingTable.NearestPeers(keyBytes, d.config.ReplicationFactor)

	var wg sync.WaitGroup
	for _, peer := range closestPeers {
		wg.Add(1)
		go func(nodeID types.NodeID) {
			defer wg.Done()

			// 网络消息传输原始 key（对端自行哈希）
			if err := network.SendAddProvider(ctx, nodeID, key, ttl); err != nil {
				log.Debug("发送 ADD_PROVIDER 失败",
					"peer", nodeID.ShortString(),
					"key", key,
					"err", err)
			}
		}(peer)
	}

	wg.Wait()

	log.Info("已注册为 provider",
		"namespace", namespace,
		"key", key,
		"hashedKey", hashedKey[:16]+"...",
		"ttl", ttl)

	return nil
}

// GetProviders 获取指定 namespace 的 providers
//
// 从 DHT 获取 namespace 的所有 providers。
// Key 格式：dep2p/v1/sys/<namespace> 或 dep2p/v1/realm/<realmID>/<namespace>
//
// 系统域发现（无需 JoinRealm）：
//
//	GetProviders 操作在系统域查询服务提供者，不检查 Realm 成员身份。
//	这允许节点在未加入任何业务 Realm 的情况下发现基础设施服务（如 Relay、Bootstrap）。
//
// 参考：docs/04-usage/examples/advanced.md#系统域服务发现
//
// T2 修复：存储和距离计算使用 SHA256(key)，网络消息传输原始 key。
// T4 修复：实现迭代查询（alpha=3，最多10轮）
// Layer1 修复：使用 NormalizeNamespace 防止双前缀陷阱
func (d *DHT) GetProviders(ctx context.Context, namespace string) ([]discoveryif.PeerInfo, error) {
	if atomic.LoadInt32(&d.running) == 0 {
		return nil, ErrDHTClosed
	}

	if namespace == "" {
		return nil, errors.New("empty namespace")
	}

	// Layer1 修复：namespace 归一化
	parsed := NormalizeNamespace(namespace)
	key := BuildProviderKeyWithParsed(parsed, string(d.realmID))

	// T2: 使用哈希后的 key 作为存储 key
	hashedKey := HashKeyString(key)

	// 先检查本地
	localProviders := d.getProvidersLocal(hashedKey)
	result := make([]discoveryif.PeerInfo, 0, len(localProviders))
	seen := make(map[types.NodeID]struct{})

	for _, p := range localProviders {
		result = append(result, discoveryif.PeerInfo{
			ID:    p.ID,
			Addrs: types.StringsToMultiaddrs(p.Addrs),
		})
		seen[p.ID] = struct{}{}
	}

	// 从网络获取
	d.networkMu.RLock()
	network := d.network
	d.networkMu.RUnlock()

	if network == nil {
		return result, nil
	}

	// T4: 迭代查询
	metaProviders, err := d.iterativeGetProviders(ctx, key, seen)
	if err != nil {
		// 返回本地结果（可能为空）
		return result, nil
	}

	// 合并结果 + 写回本地缓存（带 TTL 元数据）
	// 同时将地址写入 AddressBook（Peerstore 类底座）
	for _, p := range metaProviders {
		result = append(result, discoveryif.PeerInfo{ID: p.ID, Addrs: types.StringsToMultiaddrs(p.Addrs)})
		d.addProviderLocalWithMeta(hashedKey, p.ID, p.Addrs, p.Timestamp, p.TTL)

		// 写入 AddressBook（支撑 Endpoint.Connect() 地址查找）
		d.writeAddrsToAddressBook(p.ID, p.Addrs)
	}

	log.Debug("获取 providers",
		"namespace", namespace,
		"count", len(result))

	return result, nil
}

// iterativeGetProviders 迭代获取 providers
//
// T4 修复：实现 Kademlia 迭代查询算法
// - alpha=3 并发
// - 最多 10 轮迭代
// - 使用 closer peers 继续查询
func (d *DHT) iterativeGetProviders(ctx context.Context, key string, seen map[types.NodeID]struct{}) ([]ProviderInfo, error) {
	const maxIterations = 10

	// T2: 距离计算使用哈希后的 key bytes
	keyBytes := HashKey(key)

	// 候选节点集合（按距离排序）
	queried := make(map[types.NodeID]struct{})
	candidates := d.routingTable.NearestPeers(keyBytes, d.config.BucketSize)

	var result []ProviderInfo
	var mu sync.Mutex

	for iteration := 0; iteration < maxIterations; iteration++ {
		// 选择下一批查询节点（排除已查询过的）
		var toQuery []types.NodeID
		for _, id := range candidates {
			if _, ok := queried[id]; !ok {
				toQuery = append(toQuery, id)
				if len(toQuery) >= d.config.Alpha {
					break
				}
			}
		}

		if len(toQuery) == 0 {
			// 没有新的候选节点
			break
		}

		log.Debug("迭代查询 GET_PROVIDERS",
			"iteration", iteration,
			"key", key[:min(20, len(key))]+"...",
			"toQuery", len(toQuery))

		// 并发查询
		type queryResult struct {
			providers   []ProviderInfo
			closerPeers []types.NodeID
		}
		resultCh := make(chan queryResult, len(toQuery))

		for _, nodeID := range toQuery {
			queried[nodeID] = struct{}{}

			go func(id types.NodeID) {
				d.networkMu.RLock()
				network := d.network
				d.networkMu.RUnlock()

				if network == nil {
					resultCh <- queryResult{}
					return
				}

				providers, closerIDs, err := network.SendGetProviders(ctx, id, key)
				if err != nil {
					log.Debug("GET_PROVIDERS 失败",
						"peer", id.ShortString(),
						"err", err)
					resultCh <- queryResult{}
					return
				}
				resultCh <- queryResult{
					providers:   providers,
					closerPeers: closerIDs,
				}
			}(nodeID)
		}

		// 收集结果
		var newCandidates []types.NodeID
		for i := 0; i < len(toQuery); i++ {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case qr := <-resultCh:
				// 收集 providers
				mu.Lock()
				for _, p := range qr.providers {
					if _, ok := seen[p.ID]; ok {
						continue
					}
					result = append(result, p)
					seen[p.ID] = struct{}{}
				}
				mu.Unlock()

				// 收集 closer peers
				newCandidates = append(newCandidates, qr.closerPeers...)
			}
		}

		// 合并新候选并按距离排序
		if len(newCandidates) > 0 {
			candidates = d.mergeCandidates(candidates, newCandidates, keyBytes, queried)
		}
	}

	return result, nil
}

// ============================================================================
//                              Announcer 接口实现
// ============================================================================

// Announce 通告本节点到指定命名空间
//
// 实现 discoveryif.Announcer 接口。
// 将本节点注册为指定 namespace 的 provider。
func (d *DHT) Announce(ctx context.Context, namespace string) error {
	return d.AddProvider(ctx, namespace)
}

// AnnounceWithTTL 带 TTL 的通告
//
// 实现 discoveryif.Announcer 接口。
// - ttl 参数会写入本地 provider 记录，并通过 ADD_PROVIDER 传播到网络侧缓存。
// - ttl 会被上限裁剪到 DefaultProviderTTL（24h），避免过长缓存导致撤销不及时。
func (d *DHT) AnnounceWithTTL(ctx context.Context, namespace string, ttl time.Duration) error {
	return d.addProviderWithTTL(ctx, namespace, ttl)
}

// StopAnnounce 停止通告
//
// 实现 discoveryif.Announcer 接口。
// 从本地 provider 存储中移除本节点的注册。
// Layer1 修复：使用 NormalizeNamespace 防止双前缀陷阱
func (d *DHT) StopAnnounce(namespace string) error {
	// DiscoveryService.Stop() 会用 StopAnnounce("") 表示"停止所有通告"。
	// DHT 作为系统级 announcer，应接受该语义，避免在停止阶段产生无意义错误。
	if namespace == "" {
		return nil
	}

	// Layer1 修复：namespace 归一化
	parsed := NormalizeNamespace(namespace)
	key := BuildProviderKeyWithParsed(parsed, string(d.realmID))

	// T2: 使用哈希后的 key 作为存储 key
	hashedKey := HashKeyString(key)

	// 从本地存储中移除
	d.removeProviderLocal(hashedKey, d.localID)

	// best-effort：通知网络侧尽快移除本节点的 provider 记录
	d.networkMu.RLock()
	network := d.network
	d.networkMu.RUnlock()
	if network != nil {
		// 不要阻塞调用方：后台发送撤销
		go func() {
			baseCtx := context.Background()
			if d.ctx != nil {
				baseCtx = d.ctx
			}
			ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
			defer cancel()

			keyBytes := HashKey(key)
			closestPeers := d.routingTable.NearestPeers(keyBytes, d.config.ReplicationFactor)

			var wg sync.WaitGroup
			for _, peer := range closestPeers {
				wg.Add(1)
				go func(nodeID types.NodeID) {
					defer wg.Done()
					if err := network.SendRemoveProvider(ctx, nodeID, key); err != nil {
						log.Debug("发送 REMOVE_PROVIDER 失败",
							"peer", nodeID.ShortString(),
							"key", key,
							"err", err)
					}
				}(peer)
			}
			wg.Wait()
		}()
	}

	log.Debug("停止通告",
		"namespace", namespace,
		"key", key,
		"hashedKey", hashedKey[:16]+"...")

	return nil
}

// ============================================================================
//                              Provider 记录编解码
// ============================================================================

// ProviderRecord 提供者记录
type ProviderRecord struct {
	Provider  types.NodeID
	Addrs     []string
	Timestamp time.Time
	TTL       time.Duration
}

// Encode 编码记录
func (r *ProviderRecord) Encode() []byte {
	var buf bytes.Buffer

	// Provider ID
	buf.Write(r.Provider[:])

	// Addrs count
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(r.Addrs))) //nolint:gosec // G115: 地址数量由协议限制

	// Addrs
	for _, addr := range r.Addrs {
		_ = binary.Write(&buf, binary.BigEndian, uint16(len(addr))) //nolint:gosec // G115: 地址长度由协议限制
		buf.WriteString(addr)
	}

	// Timestamp
	_ = binary.Write(&buf, binary.BigEndian, r.Timestamp.UnixNano())

	// TTL
	_ = binary.Write(&buf, binary.BigEndian, int64(r.TTL))

	return buf.Bytes()
}

// DecodeProviderRecord 解码记录
func DecodeProviderRecord(data []byte) (*ProviderRecord, error) {
	// 最小长度: 32(NodeID) + 2(addrCount) + 8(timestamp) + 8(ttl) = 50
	const minLen = 50
	if len(data) < minLen {
		return nil, errors.New("data too short: need at least 50 bytes")
	}

	r := &ProviderRecord{}
	buf := bytes.NewReader(data)

	// Provider ID
	if _, err := buf.Read(r.Provider[:]); err != nil {
		return nil, errors.New("failed to read provider ID")
	}

	// Addrs count
	var addrCount uint16
	if err := binary.Read(buf, binary.BigEndian, &addrCount); err != nil {
		return nil, errors.New("failed to read address count")
	}

	// 防止过大的地址数量导致 OOM
	const maxAddrs = 100
	if addrCount > maxAddrs {
		return nil, errors.New("too many addresses")
	}

	// Addrs
	r.Addrs = make([]string, addrCount)
	for i := uint16(0); i < addrCount; i++ {
		var addrLen uint16
		if err := binary.Read(buf, binary.BigEndian, &addrLen); err != nil {
			return nil, errors.New("failed to read address length")
		}

		// 防止过长的地址导致 OOM
		const maxAddrLen = 1024
		if addrLen > maxAddrLen {
			return nil, errors.New("address too long")
		}

		// 检查剩余数据是否足够
		if buf.Len() < int(addrLen)+16 { // 16 = timestamp(8) + ttl(8)
			return nil, errors.New("data truncated")
		}

		addrBytes := make([]byte, addrLen)
		if _, err := buf.Read(addrBytes); err != nil {
			return nil, errors.New("failed to read address data")
		}
		r.Addrs[i] = string(addrBytes)
	}

	// Timestamp
	var ts int64
	if err := binary.Read(buf, binary.BigEndian, &ts); err != nil {
		return nil, errors.New("failed to read timestamp")
	}
	r.Timestamp = time.Unix(0, ts)

	// TTL
	var ttl int64
	if err := binary.Read(buf, binary.BigEndian, &ttl); err != nil {
		return nil, errors.New("failed to read TTL")
	}
	r.TTL = time.Duration(ttl)

	return r, nil
}

