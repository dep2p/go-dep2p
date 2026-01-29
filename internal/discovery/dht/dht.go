package dht

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	corepeerstore "github.com/dep2p/go-dep2p/internal/core/peerstore"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var logger = log.Logger("discovery/dht")

// DHT Kademlia DHT 实现
type DHT struct {
	// host 网络主机
	host pkgif.Host

	// peerstore 节点信息存储（用于缓存查询结果）
	peerstore pkgif.Peerstore

	// eventBus 事件总线（可选，用于监听连接事件）
	eventBus pkgif.EventBus

	// config 配置
	config *Config

	// routingTable 路由表
	routingTable *RoutingTable

	// valueStore 值存储
	valueStore *ValueStore

	// providerStore Provider 存储
	providerStore *ProviderStore

	// peerRecordStore PeerRecord 存储（用于权威目录）
	peerRecordStore *PeerRecordStore

	// localRecordManager 本地 PeerRecord 管理器（管理 seq 递增和续期）
	localRecordManager *LocalPeerRecordManager

	// addressChangeDetector 地址变化检测器
	addressChangeDetector *AddressChangeDetector

	// addressBookProvider 地址簿提供者（用于 DHT/Relay 协作）
	addressBookProvider AddressBookProvider

	// reachabilityChecker 可达性检测器（用于发布前验证）
	reachabilityChecker ReachabilityChecker

	// providerCache Provider 查询结果缓存
	// v2.0.1: 缓存 DHT Provider 查询结果，减少重复查询
	providerCache *ProviderCache

	// network 网络适配器
	network *NetworkAdapter

	// handler 协议处理器
	handler *Handler

	// 生命周期
	ctx       context.Context
	ctxCancel context.CancelFunc
	started   atomic.Bool
	wg        sync.WaitGroup

	mu sync.RWMutex
}

// SetEventBus 设置事件总线（可选）
// 设置后，DHT 会自动监听连接事件，将连接的节点添加到路由表
func (d *DHT) SetEventBus(eb pkgif.EventBus) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.eventBus = eb
}

// New 创建 DHT 实例
func New(host pkgif.Host, peerstore pkgif.Peerstore, opts ...ConfigOption) (*DHT, error) {
	if host == nil {
		return nil, ErrNilHost
	}

	// 应用配置选项
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	localID := types.NodeID(host.ID())

	dht := &DHT{
		host:            host,
		peerstore:       peerstore,
		config:          config,
		routingTable:    NewRoutingTable(localID),
		valueStore:      NewValueStore(),
		providerStore:   NewProviderStore(),
		peerRecordStore: NewPeerRecordStore(),
		providerCache:   NewProviderCache(), // v2.0.1: Provider 查询缓存
		ctx:             ctx,
		ctxCancel:       cancel,
	}

	// 创建网络适配器
	dht.network = NewNetworkAdapter(host, dht.routingTable, peerstore)

	// 创建本地 PeerRecord 管理器
	dht.localRecordManager = NewLocalPeerRecordManager()

	// 创建地址变化检测器
	dht.addressChangeDetector = NewAddressChangeDetector(func(oldAddrs, newAddrs []string) {
		logger.Info("检测到地址变化",
			"oldCount", len(oldAddrs),
			"newCount", len(newAddrs))
	})

	// 创建协议处理器
	dht.handler = NewHandler(dht)

	return dht, nil
}

// Start 启动 DHT
func (d *DHT) Start(ctx context.Context) error {
	if d.started.Load() {
		return ErrAlreadyStarted
	}

	logger.Info("正在启动 DHT")

	// 注册协议处理器
	d.host.SetStreamHandler(ProtocolID, func(stream pkgif.Stream) {
		d.handler.HandleStream(ctx, stream)
	})

	d.started.Store(true)

	// 启动后台循环
	d.wg.Add(3)
	go d.refreshLoop()
	go d.cleanupLoop()
	go d.republishLoop() // PeerRecord 续期循环

	// v2.0.1: 启动 Provider 缓存清理循环
	if d.providerCache != nil {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.providerCache.StartCleanupLoop(d.ctx, 1*time.Minute)
		}()
	}

	// 订阅连接事件
	d.subscribeConnectionEvents()

	logger.Info("DHT 启动成功")
	return nil
}

// subscribeConnectionEvents 订阅连接事件
func (d *DHT) subscribeConnectionEvents() {
	d.mu.RLock()
	eb := d.eventBus
	d.mu.RUnlock()

	if eb == nil {
		logger.Debug("EventBus 未设置，DHT 不会自动处理连接事件")
		return
	}

	// 订阅连接事件（必须使用指针类型）
	sub, err := eb.Subscribe(new(types.EvtPeerConnected))
	if err != nil {
		logger.Warn("订阅连接事件失败", "error", err)
		return
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer sub.Close()

		for {
			select {
			case <-d.ctx.Done():
				return
			case event := <-sub.Out():
				// 处理连接事件
				d.handlePeerConnected(event)
			}
		}
	}()

	logger.Debug("DHT 已订阅连接事件")
}

// handlePeerConnected 处理节点连接事件
func (d *DHT) handlePeerConnected(event interface{}) {
	if !d.started.Load() {
		return
	}

	// 尝试从事件中提取 PeerID
	var peerID types.PeerID

	switch e := event.(type) {
	case types.EvtPeerConnected:
		peerID = e.PeerID
	case *types.EvtPeerConnected:
		peerID = e.PeerID
	case types.PeerID:
		peerID = e
	case string:
		peerID = types.PeerID(e)
	default:
		logger.Debug("未知的连接事件类型", "type", fmt.Sprintf("%T", event))
		return
	}

	if peerID == "" {
		return
	}

	peerIDShort := string(peerID)
	if len(peerIDShort) > 8 {
		peerIDShort = peerIDShort[:8]
	}

	// 获取节点地址
	var addrs []string
	if d.network != nil && d.network.peerstore != nil {
		multiaddrs := d.network.peerstore.Addrs(peerID)
		for _, ma := range multiaddrs {
			addrs = append(addrs, ma.String())
		}
	}

	if len(addrs) == 0 {
		logger.Debug("连接的节点没有地址，无法添加到 DHT", "peerID", peerIDShort)
		return
	}

	// 计算 XOR 距离（用于理解路由表位置）
	localID := types.NodeID(d.host.ID())
	nodeID := types.NodeID(peerID)
	xorDistance := XORDistance(localID, nodeID)
	bucketIndex := BucketIndex(localID, nodeID)

	node := &RoutingNode{
		ID:       nodeID,
		Addrs:    addrs,
		LastSeen: time.Now(),
	}

	if d.routingTable.Add(node) {
		rtSize := d.routingTable.Size()
		logger.Info("节点已添加到 DHT 路由表",
			"peerID", peerIDShort,
			"addrCount", len(addrs),
			"xorDistance", fmt.Sprintf("%x", xorDistance[:4]),
			"bucketIndex", bucketIndex,
			"routingTableSize", rtSize)

		// 异步发送 PING 来交换路由信息
		go d.pingPeerAsync(peerID)
	} else {
		// 节点已存在，更新最后见时间
		d.routingTable.Update(node)
		logger.Debug("更新 DHT 路由表中的节点", "peerID", peerIDShort)
	}
}

// pingPeerAsync 异步 PING 节点
func (d *DHT) pingPeerAsync(peerID types.PeerID) {
	if !d.started.Load() {
		return
	}

	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()

	rtt, err := d.network.Ping(ctx, peerID)
	if err != nil {
		logger.Debug("DHT PING 失败", "peerID", peerID, "error", err)
		// PING 失败不移除节点，可能是协议不支持
		return
	}

	logger.Debug("DHT PING 成功", "peerID", peerID, "rtt", rtt)

	// 更新 RTT
	node := d.routingTable.Get(types.NodeID(peerID))
	if node != nil {
		node.RTT = rtt
		node.LastSeen = time.Now()
		d.routingTable.Update(node)
	}
}

// Stop 停止 DHT
func (d *DHT) Stop(_ context.Context) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	logger.Info("正在停止 DHT")

	d.started.Store(false)
	d.ctxCancel()

	// 等待后台循环结束
	d.wg.Wait()

	// 关闭网络适配器
	if err := d.network.Close(); err != nil {
		logger.Warn("关闭 DHT 网络适配器失败", "error", err)
		return err
	}

	logger.Info("DHT 已停止")
	return nil
}

// ============================================================================
//                              Discovery 接口实现
// ============================================================================

// FindPeers 发现节点（实现 Discovery 接口）
func (d *DHT) FindPeers(ctx context.Context, ns string, opts ...pkgif.DiscoveryOption) (<-chan types.PeerInfo, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	// 解析选项
	options := &pkgif.DiscoveryOptions{Limit: 20}
	for _, opt := range opts {
		opt(options)
	}

	// 将 namespace 转换为 Key（优先使用 Realm 隔离）
	realmID, realmReady := d.getLocalRealmID()
	var key string
	if realmReady {
		key = RealmProviderKey(realmID, ns)
	} else {
		key = SystemKey("provider", []byte(ns))
	}

	// v2.0.1: 先检查缓存
	if d.providerCache != nil {
		if cached, ok := d.providerCache.Get(key); ok && len(cached) > 0 {
			logger.Debug("DHT FindPeers 命中缓存",
				"namespace", ns,
				"cachedCount", len(cached))
			return d.sendCachedProviders(ctx, cached, options.Limit), nil
		}
	}

	logger.Info("DHT FindPeers 开始",
		"namespace", ns,
		"limit", options.Limit,
		"routingTableSize", d.routingTable.Size())

	ch := make(chan types.PeerInfo, options.Limit)

	go func() {
		defer close(ch)

		// 执行迭代查询（多跳，GET_PROVIDERS）
		targetID := types.NodeID(string(HashKey(key)))
		q := newIterativeQuery(d, targetID, MessageTypeGetProviders, key)
		if err := q.Run(ctx); err != nil {
			// 区分空路由表（启动阶段的正常状态）和真正的查询失败
			if errors.Is(err, ErrNoNearbyPeers) {
				logger.Debug("DHT FindPeers 跳过（路由表为空，等待节点连接）",
					"namespace", ns,
					"routingTableSize", d.routingTable.Size())
			} else {
				logger.Warn("DHT FindPeers 迭代查询失败", "namespace", ns, "error", err)
			}
			return
		}

		// 获取 Providers 结果
		peers := q.GetProviders()

		// v2.0.1: 缓存查询结果
		if d.providerCache != nil && len(peers) > 0 {
			d.providerCache.Set(key, peers)
			logger.Debug("DHT FindPeers 结果已缓存",
				"namespace", ns,
				"count", len(peers))
		}

		if len(peers) > options.Limit {
			peers = peers[:options.Limit]
		}

		// 记录找到的节点
		for i, peer := range peers {
			peerIDShort := string(peer.ID)
			if len(peerIDShort) > 8 {
				peerIDShort = peerIDShort[:8]
			}
			logger.Debug("DHT 返回节点",
				"index", i,
				"peerID", peerIDShort,
				"addrCount", len(peer.Addrs))

			// 写入 Peerstore 缓存
			d.cacheToPerstore(peer)

			select {
			case ch <- peer:
			case <-ctx.Done():
				logger.Debug("DHT FindPeers 被取消", "namespace", ns)
				return
			}
		}
	}()

	return ch, nil
}

// sendCachedProviders 发送缓存的 Provider 结果
//
// v2.0.1: 用于从缓存返回 Provider 查询结果
func (d *DHT) sendCachedProviders(ctx context.Context, providers []types.PeerInfo, limit int) <-chan types.PeerInfo {
	ch := make(chan types.PeerInfo, limit)

	go func() {
		defer close(ch)

		count := 0
		for _, peer := range providers {
			if count >= limit {
				break
			}

			// 写入 Peerstore 缓存
			d.cacheToPerstore(peer)

			select {
			case ch <- peer:
				count++
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}

// Advertise 宣告节点（实现 Discovery 接口）
//
// 之前版本只添加到本地 providerStore，没有广播到 DHT 网络。
// 现在调用 Provide(key, true) 来执行完整的广播流程：
//  1. 根据地址发布策略过滤地址
//  2. 添加到本地 providerStore
//  3. 向最近的 K 个节点广播 ADD_PROVIDER 消息
func (d *DHT) Advertise(ctx context.Context, ns string, _ ...pkgif.DiscoveryOption) (time.Duration, error) {
	if !d.started.Load() {
		return 0, ErrNotStarted
	}

	// 将 namespace 转换为 Key（与 FindPeers 保持一致）
	key := SystemKey("provider", []byte(ns))
	if realmID, ok := d.getLocalRealmID(); ok {
		key = RealmProviderKey(realmID, ns)
	}

	//调用 Provide 执行完整的广播流程
	// announce=true 表示需要向 DHT 网络广播 ADD_PROVIDER 消息
	if err := d.Provide(ctx, key, true); err != nil {
		logger.Warn("Advertise 广播失败", "namespace", ns, "key", key[:16], "error", err)
		// 即使广播失败，仍返回 TTL（本地记录已添加）
	}

	return d.config.ProviderTTL, nil
}

// UpdateAddrs 更新 DHT 中本节点的地址
//
// 当网络变化时调用，重新获取本地地址并更新 DHT 中的所有记录。
// 这确保其他节点能发现本节点的新地址。
func (d *DHT) UpdateAddrs(_ context.Context) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	// 获取当前地址（使用 AdvertisedAddrs 包含 Relay 地址）
	//NAT 后节点需要发布 Relay 地址才能被联系
	localAddrs := d.host.AdvertisedAddrs()
	localID := types.NodeID(d.host.ID())

	// 更新本地 Provider 记录
	// 遍历所有已注册的 namespace，更新地址
	d.providerStore.UpdateLocalAddrs(localID, localAddrs, d.config.ProviderTTL)

	return nil
}

// ============================================================================
//                              DHT 接口实现
// ============================================================================

// GetValue 获取值
//
// 实现流程：
//  1. 从本地 valueStore 查找
//  2. 如果未找到，执行迭代查询（FIND_VALUE）
//  3. 缓存查询结果并返回
func (d *DHT) GetValue(ctx context.Context, key string) ([]byte, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	// 1. 先从本地查找
	value, exists := d.valueStore.Get(key)
	if exists {
		return value, nil
	}

	// 2. 创建迭代查询
	target := types.NodeID(string(HashKey(key)))
	q := newIterativeQuery(d, target, MessageTypeFindValue, key)

	// 3. 执行查询
	if err := q.Run(ctx); err != nil {
		return nil, err
	}

	// 4. 检查是否找到值
	if foundValue := q.GetValue(); foundValue != nil {
		// 缓存到本地
		d.valueStore.Put(key, foundValue, d.config.MaxRecordAge)
		return foundValue, nil
	}

	return nil, ErrKeyNotFound
}

// PutValue 存储值
//
// 实现流程：
//  1. 存储到本地 valueStore
//  2. 查找最近的 K 个节点
//  3. 并发发送 STORE 请求到这些节点
func (d *DHT) PutValue(ctx context.Context, key string, value []byte) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	// 1. 存储到本地
	d.valueStore.Put(key, value, d.config.MaxRecordAge)

	// 2. 查找最近的 K 个节点
	target := types.NodeID(string(HashKey(key)))
	closestPeers := d.routingTable.NearestPeers(target, BucketSize)

	if len(closestPeers) == 0 {
		// 仅本地存储
		return nil
	}

	// 3. 并发复制到远程节点
	var wg sync.WaitGroup
	localID := types.NodeID(d.host.ID())
	//发送可被联系的地址（包含 Relay）
	localAddrs := d.host.AdvertisedAddrs()
	requestID := uint64(time.Now().UnixNano())

	for _, peer := range closestPeers {
		wg.Add(1)
		go func(p *RoutingNode) {
			defer wg.Done()

			// 构造 STORE 请求
			msg := NewStoreRequest(requestID, localID, localAddrs, key, value, uint32(d.config.MaxRecordAge.Seconds()))
			_, err := d.network.SendMessage(ctx, types.PeerID(p.ID), msg)
			if err != nil {
				// 复制失败，记录但继续
				_ = err
			}
		}(peer)
	}

	// 等待所有复制完成（最多等待 5 秒）
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		// 超时，但不返回错误（部分复制也是成功的）
		return nil
	}
}

// FindPeer 查找特定节点
//
// 实现流程：
//  1. 从本地路由表查找
//  2. 如果未找到，执行迭代查询（FIND_NODE）
//  3. 返回最近的节点信息
//  4. 将结果写入 Peerstore 缓存
//
// 方法签名与 pkgif.DHT 接口保持一致（接受 string 类型）
func (d *DHT) FindPeer(ctx context.Context, peerIDStr string) (types.PeerInfo, error) {
	if !d.started.Load() {
		return types.PeerInfo{}, ErrNotStarted
	}

	peerID := types.PeerID(peerIDStr)
	nodeID := types.NodeID(peerID)

	realmID, realmReady := d.getLocalRealmID()

	// 优先级 1: Peerstore 本地缓存（不包含 MemberList/DHT/Relay 来源）
	if addrs := d.getPeerstoreAddrsBySource(peerID,
		corepeerstore.SourcePeerstore,
		corepeerstore.SourceManual,
		corepeerstore.SourceUnknown,
	); len(addrs) > 0 {
		return types.PeerInfo{ID: peerID, Addrs: addrs}, nil
	}
	if !d.hasPeerstoreSourceAccess() && d.peerstore != nil {
		if addrs := d.peerstore.Addrs(peerID); len(addrs) > 0 {
			return types.PeerInfo{ID: peerID, Addrs: addrs}, nil
		}
	}

	// 优先级 2: MemberList（Gossip 同步）
	if addrs := d.getPeerstoreAddrsBySource(peerID, corepeerstore.SourceMemberList); len(addrs) > 0 {
		return types.PeerInfo{ID: peerID, Addrs: addrs}, nil
	}

	// 优先级 3: DHT（权威目录）
	if realmReady {
		//使用内部方法获取结构化记录
		record, err := d.getPeerRecordInternal(ctx, realmID, nodeID)
		if err == nil && record != nil && record.Record != nil {
			addrs := record.Record.AllAddrs()
			if len(addrs) > 0 {
				return types.PeerInfo{
					ID:    types.PeerID(nodeID),
					Addrs: convertToMultiaddrs(addrs),
				}, nil
			}
		}
	}

	// 优先级 4: Relay 地址簿（仅在 DHT 失败时回退）
	if d.addressBookProvider != nil {
		directAddrs, relayAddrs, found := d.addressBookProvider.GetPeerAddresses(ctx, nodeID)
		if found && (len(directAddrs) > 0 || len(relayAddrs) > 0) {
			addrs := append(directAddrs, relayAddrs...)
			return types.PeerInfo{
				ID:    types.PeerID(nodeID),
				Addrs: convertToMultiaddrs(addrs),
			}, nil
		}
	}

	if realmReady {
		return types.PeerInfo{}, ErrPeerNotFound
	}

	// 1. 从路由表查找
	node := d.routingTable.Get(types.NodeID(peerID))
	if node != nil {
		peerInfo := types.PeerInfo{
			ID:    types.PeerID(node.ID),
			Addrs: convertToMultiaddrs(node.Addrs),
		}
		// 写入 Peerstore 缓存
		d.cacheToPerstore(peerInfo)
		return peerInfo, nil
	}

	// 2. 执行迭代查询
	target := types.NodeID(peerID)
	q := newIterativeQuery(d, target, MessageTypeFindNode, "")

	if err := q.Run(ctx); err != nil {
		return types.PeerInfo{}, err
	}

	// 3. 从结果中查找目标节点
	for _, resultNode := range q.GetResult() {
		if resultNode.ID == target {
			peerInfo := types.PeerInfo{
				ID:    types.PeerID(resultNode.ID),
				Addrs: convertToMultiaddrs(resultNode.Addrs),
			}
			// 写入 Peerstore 缓存
			d.cacheToPerstore(peerInfo)
			return peerInfo, nil
		}
	}

	return types.PeerInfo{}, ErrPeerNotFound
}

// cacheToPerstore 将查询结果写入 Peerstore 缓存
//
// # DHT 查询结果自动缓存到 Peerstore，加速后续连接
//
// 跳过本地节点，避免 "dial to self" 错误
func (d *DHT) cacheToPerstore(peerInfo types.PeerInfo) {
	if d.peerstore == nil {
		return
	}

	// 跳过本地节点
	if peerInfo.ID == types.PeerID(d.host.ID()) {
		return
	}

	if len(peerInfo.Addrs) == 0 {
		return
	}

	// 使用 PeerRecordTTL 作为缓存 TTL
	ttl := d.config.PeerRecordTTL
	if ttl == 0 {
		ttl = DefaultPeerRecordTTL
	}

	// 添加地址到 Peerstore（标记来源为 DHT）
	if writer, ok := d.peerstore.(interface {
		AddAddrsWithSource(types.PeerID, []corepeerstore.AddressWithSource)
	}); ok {
		var addrsWithSource []corepeerstore.AddressWithSource
		for _, addr := range peerInfo.Addrs {
			addrsWithSource = append(addrsWithSource, corepeerstore.AddressWithSource{
				Addr:   addr,
				Source: corepeerstore.SourceDHT,
				TTL:    ttl,
			})
		}
		writer.AddAddrsWithSource(peerInfo.ID, addrsWithSource)
	} else {
		d.peerstore.AddAddrs(peerInfo.ID, peerInfo.Addrs, ttl)
	}

	logger.Debug("DHT 查询结果已缓存到 Peerstore",
		"peerID", peerInfo.ID.ShortString(),
		"addrCount", len(peerInfo.Addrs),
		"ttl", ttl)
}

// Provide 宣告内容提供者
//
// 根据地址发布策略过滤地址
// - PublishAuto: 自动根据可达性决定（Private 节点只发布 relay_addrs）
// - PublishAll: 发布所有地址
// - PublishDirectOnly: 仅发布直连地址
// - PublishRelayOnly: 仅发布中继地址
//
// 实现流程：
//  1. 根据策略过滤地址
//  2. 添加到本地 providerStore
//  3. 如果 announce=true，广播到最近的 K 个节点
func (d *DHT) Provide(ctx context.Context, key string, announce bool) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	// 优先使用 Realm 隔离 Key
	key = d.normalizeProviderKey(key)

	ttl := d.config.ProviderTTL
	localID := types.NodeID(d.host.ID())

	// 根据策略过滤地址
	//使用 AdvertisedAddrs 确保包含 Relay 地址
	localAddrs := d.filterAddressesByStrategy(d.host.AdvertisedAddrs())

	// 如果 AdvertisedAddrs 为空，回退到监听地址
	// 这在节点刚启动、Relay 地址还未准备好时提供基本的发现能力
	if len(localAddrs) == 0 {
		listenAddrs := d.host.Addrs()
		if len(listenAddrs) > 0 {
			localAddrs = d.filterAddressesByStrategy(listenAddrs)
			logger.Debug("Provide: AdvertisedAddrs 为空，回退到监听地址",
				"key", key[:8],
				"listenAddrCount", len(localAddrs))
		}
	}

	if len(localAddrs) == 0 {
		logger.Warn("Provide: 没有可发布的地址", "key", key[:8], "strategy", d.config.AddressPublishStrategy)
		// 仍然继续，使用空地址列表（可能在本地存储后其他节点来查询时有效）
	}

	// 1. 添加到本地
	d.providerStore.AddProvider(key, localID, localAddrs, ttl)

	// 2. 如果需要广播
	if announce {
		// 查找最近的 K 个节点
		target := types.NodeID(string(HashKey(key)))
		closestPeers := d.routingTable.NearestPeers(target, BucketSize)

		if len(closestPeers) == 0 {
			// 没有附近节点，仅本地存储
			return nil
		}

		// 3. 并发广播 ADD_PROVIDER 消息
		requestID := uint64(time.Now().UnixNano())
		for _, peer := range closestPeers {
			go func(p *RoutingNode) {
				msg := NewAddProviderRequest(requestID, localID, localAddrs, key, uint32(ttl.Seconds()))
				_, err := d.network.SendMessage(ctx, types.PeerID(p.ID), msg)
				if err != nil {
					// 广播失败，记录但继续
					_ = err
				}
			}(peer)
		}
	}

	return nil
}

// filterAddressesByStrategy 根据策略过滤地址
//
// 实现地址发布策略
func (d *DHT) filterAddressesByStrategy(allAddrs []string) []string {
	strategy := d.config.AddressPublishStrategy

	// 如果是自动策略，根据可达性决定
	if strategy == PublishAuto {
		if d.config.ReachabilityProvider != nil {
			natType := d.config.ReachabilityProvider()
			if isPrivateNAT(natType) {
				strategy = PublishRelayOnly
				logger.Debug("PublishAuto: 节点为 Private，使用 RelayOnly 策略", "natType", natType)
			} else {
				strategy = PublishAll
				logger.Debug("PublishAuto: 节点为 Public，使用 PublishAll 策略", "natType", natType)
			}
		} else {
			// 没有可达性提供器，默认 PublishAll
			strategy = PublishAll
		}
	}

	switch strategy {
	case PublishAll:
		return allAddrs
	case PublishDirectOnly:
		return filterDirectAddrs(allAddrs)
	case PublishRelayOnly:
		return filterRelayAddrs(allAddrs)
	default:
		return allAddrs
	}
}

// isPrivateNAT 判断是否为 Private NAT 类型
func isPrivateNAT(natType types.NATType) bool {
	// NAT 类型判断
	// Private 包括：Unknown、Symmetric（对称型 NAT 通常难以穿透）
	// Public 包括：None（公网）、FullCone、RestrictedCone、PortRestricted
	return natType == types.NATTypeUnknown ||
		natType == types.NATTypeSymmetric
}

// filterDirectAddrs 过滤出直连地址（非 Relay）
func filterDirectAddrs(addrs []string) []string {
	var direct []string
	for _, addr := range addrs {
		if !isRelayAddr(addr) {
			direct = append(direct, addr)
		}
	}
	return direct
}

// filterRelayAddrs 过滤出中继地址
func filterRelayAddrs(addrs []string) []string {
	var relay []string
	for _, addr := range addrs {
		if isRelayAddr(addr) {
			relay = append(relay, addr)
		}
	}
	return relay
}

// isRelayAddr 判断是否为 Relay 地址
func isRelayAddr(addr string) bool {
	return strings.Contains(addr, "/p2p-circuit/")
}

// truncateKey 安全截取 key 用于日志显示
func truncateKey(key string) string {
	if len(key) <= 16 {
		return key
	}
	return key[:16] + "..."
}

// FindProviders 查找内容提供者
func (d *DHT) FindProviders(ctx context.Context, key string) (<-chan types.PeerInfo, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	// 优先使用 Realm 隔离 Key
	key = d.normalizeProviderKey(key)

	ch := make(chan types.PeerInfo, 100)

	go func() {
		defer close(ch)

		seen := make(map[types.PeerID]struct{})

		sendProvider := func(info types.PeerInfo) bool {
			if info.ID == "" {
				return true
			}
			if _, exists := seen[info.ID]; exists {
				return true
			}
			seen[info.ID] = struct{}{}
			// 写入 Peerstore 缓存
			d.cacheToPerstore(info)
			select {
			case ch <- info:
				return true
			case <-ctx.Done():
				return false
			}
		}

		// 1. 本地 Provider 缓存
		providers := d.providerStore.GetProviders(key)

		for _, p := range providers {
			info := types.PeerInfo{
				ID:    p.PeerID,
				Addrs: convertToMultiaddrs(p.Addrs),
			}
			if !sendProvider(info) {
				return
			}
		}

		//
		// 这可以大幅减少网络开销（实测 27 分钟可减少 600+ 次 DHT 查询）
		if d.providerCache != nil {
			if cached, ok := d.providerCache.Get(key); ok && len(cached) > 0 {
				logger.Debug("FindProviders 命中缓存，跳过 DHT 查询",
					"key", truncateKey(key),
					"cachedCount", len(cached))
				for _, info := range cached {
					if !sendProvider(info) {
						return
					}
				}
				return // 缓存命中，无需 DHT 查询
			}
		}

		// 2. DHT 迭代查询（多跳）
		target := types.NodeID(string(HashKey(key)))
		q := newIterativeQuery(d, target, MessageTypeGetProviders, key)
		if err := q.Run(ctx); err != nil {
			// OPT-1 扩展：空路由表时降级为 DEBUG（启动阶段的正常状态）
			if errors.Is(err, ErrNoNearbyPeers) {
				logger.Debug("DHT FindProviders 跳过（路由表为空，等待节点连接）",
					"key", key,
					"routingTableSize", d.routingTable.Size())
			} else {
				logger.Warn("DHT FindProviders 迭代查询失败", "key", key, "error", err)
			}
			return
		}

		queryResults := q.GetProviders()

		//
		if d.providerCache != nil && len(queryResults) > 0 {
			d.providerCache.Set(key, queryResults)
			logger.Debug("FindProviders 结果已缓存",
				"key", truncateKey(key),
				"count", len(queryResults))
		}

		for _, info := range queryResults {
			if !sendProvider(info) {
				return
			}
		}
	}()

	return ch, nil
}

// Bootstrap 引导 DHT
//
// 引导节点来源合并：
//  1. 配置中的 BootstrapPeers（优先）
//  2. Peerstore 中已知的 peers（补充）
func (d *DHT) Bootstrap(ctx context.Context) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	// 收集引导节点（配置 + Peerstore 合并）
	bootstrapPeers := d.collectBootstrapPeers()

	if len(bootstrapPeers) == 0 {
		logger.Warn("DHT Bootstrap: 无引导节点可用（配置和 Peerstore 均无）")
		return nil
	}

	logger.Info("DHT Bootstrap 开始",
		"peerCount", len(bootstrapPeers),
		"routingTableSize", d.routingTable.Size())

	successCount := 0
	failCount := 0

	// 连接到引导节点
	for _, peer := range bootstrapPeers {
		peerIDShort := string(peer.ID)
		if len(peerIDShort) > 8 {
			peerIDShort = peerIDShort[:8]
		}
		addrs := convertToStrings(peer.Addrs)

		logger.Debug("DHT Bootstrap: 尝试连接引导节点",
			"peerID", peerIDShort,
			"addrCount", len(addrs),
			"source", peer.Source)

		if err := d.host.Connect(ctx, string(peer.ID), addrs); err != nil {
			failCount++
			logger.Warn("DHT Bootstrap: 连接引导节点失败",
				"peerID", peerIDShort,
				"error", err)
			continue // 忽略失败，继续尝试其他节点
		}

		// 添加到路由表
		added := d.routingTable.Add(&RoutingNode{
			ID:       types.NodeID(peer.ID),
			Addrs:    addrs,
			LastSeen: time.Now(),
		})

		successCount++
		logger.Info("DHT Bootstrap: 引导节点连接成功",
			"peerID", peerIDShort,
			"addedToRT", added,
			"routingTableSize", d.routingTable.Size())
	}

	logger.Info("DHT Bootstrap 完成",
		"success", successCount,
		"failed", failCount,
		"routingTableSize", d.routingTable.Size())

	return nil
}

// collectBootstrapPeers 收集引导节点
//
// 合并来源：
//  1. 配置中的 BootstrapPeers（优先，标记为 SourceBootstrap）
//  2. Peerstore 中已知的 peers（补充，排除本地节点）
//
// 去重规则：以 PeerID 为键，配置优先
func (d *DHT) collectBootstrapPeers() []types.PeerInfo {
	// 使用 map 去重，key 为 PeerID
	peerMap := make(map[types.PeerID]types.PeerInfo)
	localID := types.PeerID(d.host.ID())

	// 1. 从配置中的 BootstrapPeers 添加（优先）
	for _, peer := range d.config.BootstrapPeers {
		// 排除本地节点
		if peer.ID == localID {
			continue
		}
		peerMap[peer.ID] = peer
	}
	configCount := len(peerMap)

	// 2. 从 Peerstore 补充（如果有）
	peerstoreCount := 0
	if d.peerstore != nil {
		peersWithAddrs := d.peerstore.PeersWithAddrs()
		for _, peerID := range peersWithAddrs {
			// 排除本地节点
			if peerID == localID {
				continue
			}
			// 如果已在配置中存在，跳过（配置优先）
			if _, exists := peerMap[peerID]; exists {
				continue
			}
			// 获取地址
			addrs := d.peerstore.Addrs(peerID)
			if len(addrs) == 0 {
				continue
			}
			// 添加到引导列表
			peerMap[peerID] = types.PeerInfo{
				ID:     peerID,
				Addrs:  addrs,
				Source: types.SourceManual, // 从 Peerstore 获取的标记为 Manual
			}
			peerstoreCount++
		}
	}

	// 转换为切片
	result := make([]types.PeerInfo, 0, len(peerMap))
	for _, peer := range peerMap {
		result = append(result, peer)
	}

	if len(result) > 0 {
		logger.Debug("DHT 收集引导节点完成",
			"fromConfig", configCount,
			"fromPeerstore", peerstoreCount,
			"total", len(result))
	}

	return result
}

// RoutingTable 返回路由表（实现 DHT 接口）
func (d *DHT) RoutingTable() pkgif.RoutingTable {
	return &routingTableWrapper{rt: d.routingTable}
}

// ============================================================================
//                              后台循环
// ============================================================================

// refreshLoop 路由表刷新循环
func (d *DHT) refreshLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 刷新路由表
			d.routingTable.RemoveExpiredNodes()

		case <-d.ctx.Done():
			return
		}
	}
}

// cleanupLoop 清理循环
func (d *DHT) cleanupLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 清理过期数据
			d.valueStore.CleanupExpired()
			d.providerStore.CleanupExpired()

			// 清理过期 PeerRecord
			if d.peerRecordStore != nil {
				count := d.peerRecordStore.CleanupExpired()
				if count > 0 {
					logger.Debug("清理过期 PeerRecord", "count", count)
				}
			}

		case <-d.ctx.Done():
			return
		}
	}
}

// republishLoop PeerRecord 续期循环
//
// 定时检查并续期本地 PeerRecord
//
// 功能：
//  1. 定时检查是否需要重新发布（基于 RepublishInterval，默认 TTL/2）
//  2. 检测地址变化时立即重新发布
//  3. 自动递增 seq 防止重放攻击
func (d *DHT) republishLoop() {
	defer d.wg.Done()

	// 使用配置的重新发布间隔，默认为 TTL/2
	interval := d.config.RepublishInterval
	if interval == 0 {
		interval = d.config.PeerRecordTTL / 2
	}
	if interval == 0 {
		interval = DefaultPeerRecordTTL / 2
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 地址变化检测（更频繁，每分钟检测一次）
	addressCheckTicker := time.NewTicker(1 * time.Minute)
	defer addressCheckTicker.Stop()

	logger.Info("PeerRecord 续期循环已启动",
		"republishInterval", interval,
		"addressCheckInterval", "1m")

	for {
		select {
		case <-ticker.C:
			// 定时续期
			d.republishLocalPeerRecordIfNeeded("interval")

		case <-addressCheckTicker.C:
			// 地址变化检测
			d.republishLocalPeerRecordIfNeeded("address_check")

		case <-d.ctx.Done():
			logger.Debug("PeerRecord 续期循环已停止")
			return
		}
	}
}

// republishLocalPeerRecordIfNeeded 检查并续期本地 PeerRecord
//
// 根据条件决定是否重新发布
func (d *DHT) republishLocalPeerRecordIfNeeded(trigger string) {
	if d.localRecordManager == nil || !d.localRecordManager.IsInitialized() {
		return
	}

	// 获取当前地址（使用 AdvertisedAddrs 包含 Relay 地址）
	//NAT 后节点必须发布 Relay 地址
	allAddrs := d.host.AdvertisedAddrs()
	directAddrs := filterDirectAddrs(allAddrs)
	relayAddrs := filterRelayAddrs(allAddrs)

	// 检查是否需要重新发布
	needsRepublish, reason := d.localRecordManager.NeedsRepublish(
		d.config.RepublishInterval,
		directAddrs,
		relayAddrs,
	)

	if !needsRepublish {
		return
	}

	logger.Debug("开始重新发布 PeerRecord",
		"trigger", trigger,
		"reason", reason,
		"directAddrs", len(directAddrs),
		"relayAddrs", len(relayAddrs))

	//优先使用带验证的发布方法
	// 如果配置了 ReachabilityChecker，使用 PublishLocalPeerRecordWithVerification
	// 这样可以确保只发布经过验证的可达地址
	var err error
	var decision *PublishDecision

	if d.reachabilityChecker != nil {
		decision, err = d.PublishLocalPeerRecordWithVerification(d.ctx)
		if err != nil {
			logger.Warn("重新发布 PeerRecord 失败（带验证）", "error", err)
		} else if decision != nil {
			logger.Info("PeerRecord 续期成功（带验证）",
				"trigger", trigger,
				"reason", decision.Reason,
				"directCount", len(decision.DirectAddrs),
				"relayCount", len(decision.RelayAddrs),
				"seq", d.localRecordManager.CurrentSeq())
			if len(decision.Warnings) > 0 {
				logger.Warn("PeerRecord 发布警告", "warnings", decision.Warnings)
			}
		}
	} else {
		// 回退到无验证的发布方法
		err = d.PublishLocalPeerRecord(d.ctx)
		if err != nil {
			logger.Warn("重新发布 PeerRecord 失败", "error", err)
		} else {
			logger.Info("PeerRecord 续期成功",
				"trigger", trigger,
				"reason", reason,
				"seq", d.localRecordManager.CurrentSeq())
		}
	}
}

// ============================================================================
//                              辅助函数
// ============================================================================

// convertToMultiaddrs 转换字符串地址为 Multiaddr
func convertToMultiaddrs(addrs []string) []types.Multiaddr {
	result := make([]types.Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		ma, err := types.NewMultiaddr(addr)
		if err == nil {
			result = append(result, ma)
		}
	}
	return result
}

// convertToStrings 转换 Multiaddr 为字符串
func convertToStrings(addrs []types.Multiaddr) []string {
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = addr.String()
	}
	return result
}

// routingTableWrapper 路由表包装器（适配 interfaces.RoutingTable）
type routingTableWrapper struct {
	rt *RoutingTable
}

func (rtw *routingTableWrapper) Size() int {
	return rtw.rt.Size()
}

func (rtw *routingTableWrapper) NearestPeers(id string, count int) []string {
	nodes := rtw.rt.NearestPeers(types.NodeID(id), count)
	result := make([]string, len(nodes))
	for i, node := range nodes {
		result[i] = string(node.ID)
	}
	return result
}

func (rtw *routingTableWrapper) Update(id string) error {
	// 简单实现：更新LastSeen时间
	node := rtw.rt.Get(types.NodeID(id))
	if node != nil {
		node.LastSeen = time.Now()
		rtw.rt.Update(node)
	}
	return nil
}

func (rtw *routingTableWrapper) Remove(id string) {
	rtw.rt.Remove(types.NodeID(id))
}

// ============================================================================
//                              PeerRecord 操作// ============================================================================

// PublishPeerRecord 发布签名的 PeerRecord 到 DHT
//
// 方法签名与 pkgif.DHT 接口保持一致（接受序列化字节）
//
// 实现流程：
//  1. 反序列化 PeerRecord
//  2. 验证 PeerRecord
//  3. 存储到本地 peerRecordStore
//  4. 复制到最近的 K 个节点
//
// 参数:
//   - ctx: 上下文
//   - record: 签名的 PeerRecord（序列化字节）
//
// 返回:
//   - error: 发布失败时返回错误
func (d *DHT) PublishPeerRecord(ctx context.Context, record []byte) error {
	if len(record) == 0 {
		return ErrNilPeerRecord
	}
	// 反序列化
	signed, err := UnmarshalSignedRealmPeerRecord(record)
	if err != nil {
		return fmt.Errorf("failed to unmarshal peer record: %w", err)
	}
	return d.publishPeerRecordInternal(ctx, signed)
}

// publishPeerRecordInternal 发布签名的 PeerRecord 到 DHT（内部实现）
//
// 实现流程：
//  1. 验证 PeerRecord
//  2. 存储到本地 peerRecordStore
//  3. 复制到最近的 K 个节点
//
// 参数:
//   - ctx: 上下文
//   - signed: 签名的 PeerRecord
//
// 返回:
//   - error: 发布失败时返回错误
func (d *DHT) publishPeerRecordInternal(ctx context.Context, signed *SignedRealmPeerRecord) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	if signed == nil || signed.Record == nil {
		return ErrNilPeerRecord
	}

	// 构造 Key
	key := RealmPeerKey(signed.Record.RealmID, signed.Record.NodeID)

	// 1. 存储到本地
	replaced, err := d.peerRecordStore.Put(key, signed)
	if err != nil {
		return fmt.Errorf("failed to store locally: %w", err)
	}

	logger.Debug("PeerRecord 本地存储成功",
		"nodeID", signed.Record.NodeID,
		"realmID", signed.Record.RealmID,
		"seq", signed.Record.Seq,
		"replaced", replaced)

	// 2. 序列化 SignedRealmPeerRecord
	signedBytes, err := signed.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal signed record: %w", err)
	}

	// 3. 查找最近的 K 个节点
	target := types.NodeID(string(HashKey(key)))
	closestPeers := d.routingTable.NearestPeers(target, BucketSize)

	if len(closestPeers) == 0 {
		// 仅本地存储
		logger.Debug("PeerRecord 仅本地存储（无附近节点）", "key", key[:16])
		return nil
	}

	// 4. 并发复制到远程节点
	var wg sync.WaitGroup
	localID := types.NodeID(d.host.ID())
	//发送可被联系的地址（包含 Relay）
	localAddrs := d.host.AdvertisedAddrs()
	requestID := uint64(time.Now().UnixNano())

	successCount := int32(0)

	for _, peer := range closestPeers {
		wg.Add(1)
		go func(p *RoutingNode) {
			defer wg.Done()

			// 构造 PUT_PEER_RECORD 请求
			msg := NewPutPeerRecordRequest(requestID, localID, localAddrs, key, signedBytes)
			resp, err := d.network.SendMessage(ctx, types.PeerID(p.ID), msg)
			if err != nil {
				logger.Debug("PeerRecord 复制失败", "target", p.ID, "error", err)
				return
			}

			if resp.Success {
				atomic.AddInt32(&successCount, 1)
			}
		}(peer)
	}

	// 等待所有复制完成（最多等待 10 秒）
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("PeerRecord 发布完成",
			"key", key[:16],
			"success", atomic.LoadInt32(&successCount),
			"total", len(closestPeers))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		// 超时，但部分复制也是成功的
		logger.Warn("PeerRecord 发布超时",
			"key", key[:16],
			"success", atomic.LoadInt32(&successCount),
			"total", len(closestPeers))
		return nil
	}
}

// PublishGlobalPeerRecord 发布全局节点记录到 DHT
//
// 用于 Phase A 阶段（冷启动、未加入 Realm）的全局节点发布。
// 与 PublishPeerRecord 的区别：
//   - Key 使用 GlobalPeerKey (格式: /dep2p/node/<NodeID>)
//   - 不依赖 RealmID，用于全局 DHT 发现
//
// 这是讨论稿 Step A5 中描述的 "/dep2p/node/<NodeID>" 发布实现。
//
// 参数:
//   - ctx: 上下文
//   - signed: 签名的节点记录（RealmID 可为空）
//
// 返回:
//   - error: 发布失败时返回错误
func (d *DHT) PublishGlobalPeerRecord(ctx context.Context, signed *SignedRealmPeerRecord) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	if signed == nil || signed.Record == nil {
		return ErrNilPeerRecord
	}

	// 构造全局 Key（不依赖 RealmID）
	key := GlobalPeerKey(signed.Record.NodeID)

	// 1. 存储到本地
	replaced, err := d.peerRecordStore.Put(key, signed)
	if err != nil {
		return fmt.Errorf("failed to store locally: %w", err)
	}

	logger.Debug("GlobalPeerRecord 本地存储成功",
		"nodeID", signed.Record.NodeID,
		"seq", signed.Record.Seq,
		"replaced", replaced)

	// 2. 序列化 SignedRealmPeerRecord
	signedBytes, err := signed.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal signed record: %w", err)
	}

	// 3. 查找最近的 K 个节点
	target := types.NodeID(string(HashKey(key)))
	closestPeers := d.routingTable.NearestPeers(target, BucketSize)

	if len(closestPeers) == 0 {
		// 仅本地存储
		logger.Debug("GlobalPeerRecord 仅本地存储（无附近节点）", "nodeID", signed.Record.NodeID)
		return nil
	}

	// 4. 并发复制到远程节点
	var wg sync.WaitGroup
	localID := types.NodeID(d.host.ID())
	//发送可被联系的地址（包含 Relay）
	localAddrs := d.host.AdvertisedAddrs()
	requestID := uint64(time.Now().UnixNano())

	successCount := int32(0)

	for _, peer := range closestPeers {
		wg.Add(1)
		go func(p *RoutingNode) {
			defer wg.Done()

			// 构造 PUT_PEER_RECORD 请求
			msg := NewPutPeerRecordRequest(requestID, localID, localAddrs, key, signedBytes)
			resp, err := d.network.SendMessage(ctx, types.PeerID(p.ID), msg)
			if err != nil {
				logger.Debug("GlobalPeerRecord 复制失败", "target", p.ID, "error", err)
				return
			}

			if resp.Success {
				atomic.AddInt32(&successCount, 1)
			}
		}(peer)
	}

	// 等待所有复制完成（最多等待 10 秒）
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("GlobalPeerRecord 发布完成",
			"nodeID", signed.Record.NodeID,
			"success", atomic.LoadInt32(&successCount),
			"total", len(closestPeers))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		// 超时，但部分复制也是成功的
		logger.Warn("GlobalPeerRecord 发布超时",
			"nodeID", signed.Record.NodeID,
			"success", atomic.LoadInt32(&successCount),
			"total", len(closestPeers))
		return nil
	}
}

// GetGlobalPeerRecord 从 DHT 获取全局节点记录
//
// 用于查询 Phase A 阶段发布的全局节点信息。
//
// 参数:
//   - ctx: 上下文
//   - nodeID: 节点 ID
//
// 返回:
//   - *SignedRealmPeerRecord: 找到的记录
//   - error: 查询失败时返回错误
func (d *DHT) GetGlobalPeerRecord(ctx context.Context, nodeID types.NodeID) (*SignedRealmPeerRecord, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	// 构造全局 Key
	key := GlobalPeerKey(nodeID)

	// 1. 先从本地查找
	if record, exists := d.peerRecordStore.Get(key); exists {
		logger.Debug("GlobalPeerRecord 本地命中", "nodeID", nodeID, "seq", record.Record.Seq)
		return record, nil
	}

	// 2. 执行迭代查询
	target := types.NodeID(string(HashKey(key)))
	q := newIterativeQuery(d, target, MessageTypeGetPeerRecord, key)

	if err := q.Run(ctx); err != nil {
		return nil, fmt.Errorf("iterative query failed: %w", err)
	}

	// 3. 检查是否找到 PeerRecord
	if signedBytes := q.GetValue(); signedBytes != nil {
		// 反序列化
		signed, err := UnmarshalSignedRealmPeerRecord(signedBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal peer record: %w", err)
		}

		// 验证签名
		if err := VerifySignedRealmPeerRecord(signed); err != nil {
			return nil, fmt.Errorf("peer record signature verification failed: %w", err)
		}

		// 缓存到本地 peerRecordStore
		if _, err := d.peerRecordStore.Put(key, signed); err != nil {
			logger.Warn("GlobalPeerRecord 本地缓存失败", "nodeID", nodeID, "error", err)
		}

		// 将地址缓存到 Peerstore
		d.cachePeerRecordToPeerstore(signed)

		return signed, nil
	}

	return nil, ErrPeerNotFound
}

// GetPeerRecord 从 DHT 获取 PeerRecord
//
// 实现流程：
//  1. 从本地 peerRecordStore 查找
//  2. 如果未找到，执行迭代查询
//  3. 验证并返回最优记录
//
// 参数:
//   - ctx: 上下文
//   - realmID: Realm ID
//   - nodeID: 节点 ID
//
// 返回:
//   - []byte: 签名的 PeerRecord（序列化字节）
//   - error: 查询失败时返回错误
//
// 方法签名与 pkgif.DHT 接口保持一致（返回序列化字节）
func (d *DHT) GetPeerRecord(ctx context.Context, realmID types.RealmID, nodeID types.NodeID) ([]byte, error) {
	record, err := d.getPeerRecordInternal(ctx, realmID, nodeID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrPeerNotFound
	}
	// 序列化返回
	return record.Marshal()
}

// getPeerRecordInternal 获取 PeerRecord 的内部实现
//
// 返回结构化的 SignedRealmPeerRecord，供 DHT 内部使用。
// 外部应使用 GetPeerRecord 方法获取序列化字节。
//
// 参数:
//   - ctx: 上下文
//   - realmID: Realm ID
//   - nodeID: 节点 ID
//
// 返回:
//   - *SignedRealmPeerRecord: 找到的记录
//   - error: 查询失败时返回错误
func (d *DHT) getPeerRecordInternal(ctx context.Context, realmID types.RealmID, nodeID types.NodeID) (*SignedRealmPeerRecord, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	// 构造 Key
	key := RealmPeerKey(realmID, nodeID)

	// 1. 先从本地查找
	if record, exists := d.peerRecordStore.Get(key); exists {
		logger.Debug("PeerRecord 本地命中", "nodeID", nodeID, "seq", record.Record.Seq)
		return record, nil
	}

	// 2. 执行迭代查询
	target := types.NodeID(string(HashKey(key)))
	q := newIterativeQuery(d, target, MessageTypeGetPeerRecord, key)

	if err := q.Run(ctx); err != nil {
		return nil, fmt.Errorf("iterative query failed: %w", err)
	}

	// 3. 检查是否找到 PeerRecord
	if signedBytes := q.GetValue(); signedBytes != nil {
		// 反序列化
		signed, err := UnmarshalSignedRealmPeerRecord(signedBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal peer record: %w", err)
		}

		// 验证
		validator := NewDefaultPeerRecordValidator()
		if err := validator.Validate(key, signed); err != nil {
			return nil, fmt.Errorf("peer record validation failed: %w", err)
		}

		// 缓存到本地 peerRecordStore
		if _, err := d.peerRecordStore.Put(key, signed); err != nil {
			logger.Warn("缓存 PeerRecord 失败", "error", err)
		}

		// 将地址缓存到 Peerstore
		d.cachePeerRecordToPeerstore(signed)

		return signed, nil
	}

	return nil, ErrPeerNotFound
}

// cachePeerRecordToPeerstore 将 PeerRecord 的地址缓存到 Peerstore
//
// DHT 查询 PeerRecord 后自动缓存地址到 Peerstore
func (d *DHT) cachePeerRecordToPeerstore(signed *SignedRealmPeerRecord) {
	if d.peerstore == nil || signed == nil || signed.Record == nil {
		return
	}

	record := signed.Record

	// 收集所有地址
	var allAddrs []types.Multiaddr
	for _, addr := range record.DirectAddrs {
		if ma, err := types.NewMultiaddr(addr); err == nil {
			allAddrs = append(allAddrs, ma)
		}
	}
	for _, addr := range record.RelayAddrs {
		if ma, err := types.NewMultiaddr(addr); err == nil {
			allAddrs = append(allAddrs, ma)
		}
	}

	if len(allAddrs) == 0 {
		return
	}

	// 使用记录的 TTL 作为缓存 TTL
	ttl := time.Duration(record.TTL) * time.Millisecond
	if ttl == 0 {
		ttl = d.config.PeerRecordTTL
	}
	if ttl == 0 {
		ttl = DefaultPeerRecordTTL
	}

	// 添加到 Peerstore
	peerID := types.PeerID(record.NodeID)
	if writer, ok := d.peerstore.(interface {
		AddAddrsWithSource(types.PeerID, []corepeerstore.AddressWithSource)
	}); ok {
		var addrsWithSource []corepeerstore.AddressWithSource
		for _, addr := range allAddrs {
			addrsWithSource = append(addrsWithSource, corepeerstore.AddressWithSource{
				Addr:   addr,
				Source: corepeerstore.SourceDHT,
				TTL:    ttl,
			})
		}
		writer.AddAddrsWithSource(peerID, addrsWithSource)
	} else {
		d.peerstore.AddAddrs(peerID, allAddrs, ttl)
	}

	logger.Debug("PeerRecord 地址已缓存到 Peerstore",
		"peerID", peerID.ShortString(),
		"directAddrs", len(record.DirectAddrs),
		"relayAddrs", len(record.RelayAddrs),
		"ttl", ttl)
}

// LocalPeerRecordStore 返回本地 PeerRecord 存储（用于调试和测试）
func (d *DHT) LocalPeerRecordStore() *PeerRecordStore {
	return d.peerRecordStore
}

// ============================================================================
//                              本地 PeerRecord 管理// ============================================================================

// InitializeLocalRecordManager 初始化本地 PeerRecord 管理器
//
// 必须在调用 PublishLocalPeerRecord 之前调用
//
// 参数:
//   - privKey: 用于签名的私钥（支持 crypto.PrivateKey 或 pkgif.PrivateKey）
//   - realmID: 当前 Realm ID
func (d *DHT) InitializeLocalRecordManager(privKey interface{}, realmID types.RealmID) error {
	if d.localRecordManager == nil {
		return fmt.Errorf("local record manager not initialized")
	}

	// 类型转换：支持 crypto.PrivateKey 和 pkgif.PrivateKey
	var pk crypto.PrivateKey

	switch k := privKey.(type) {
	case crypto.PrivateKey:
		// 直接使用 crypto.PrivateKey
		pk = k
	case pkgif.PrivateKey:
		// 从 pkgif.PrivateKey 转换为 crypto.PrivateKey
		// 通过 Raw() 和 Type() 重新构建
		raw, err := k.Raw()
		if err != nil {
			return fmt.Errorf("failed to get raw private key: %w", err)
		}
		keyType := crypto.KeyType(k.Type())
		pk, err = crypto.UnmarshalPrivateKey(keyType, raw)
		if err != nil {
			return fmt.Errorf("failed to unmarshal private key: %w", err)
		}
		logger.Debug("已从 pkgif.PrivateKey 转换为 crypto.PrivateKey",
			"keyType", keyType.String())
	default:
		return fmt.Errorf("invalid private key type: %T (expected crypto.PrivateKey or pkgif.PrivateKey)", privKey)
	}

	localID := types.NodeID(d.host.ID())
	d.localRecordManager.Initialize(pk, localID, realmID)

	logger.Info("本地 PeerRecord 管理器已初始化",
		"nodeID", localID,
		"realmID", realmID)

	return nil
}

// PublishLocalPeerRecord 发布本地 PeerRecord
//
// 使用 LocalPeerRecordManager 创建并发布本地 PeerRecord
//
// 功能：
//  1. 收集当前地址
//  2. 根据配置的策略过滤地址
//  3. 创建带签名的 PeerRecord（自动递增 seq）
//  4. 发布到 DHT
//
// 参数:
//   - ctx: 上下文
//
// 返回:
//   - error: 发布失败时返回错误
func (d *DHT) PublishLocalPeerRecord(ctx context.Context) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	if d.localRecordManager == nil || !d.localRecordManager.IsInitialized() {
		return fmt.Errorf("local record manager not initialized, call InitializeLocalRecordManager first")
	}

	// 1. 获取当前地址（使用 AdvertisedAddrs 包含 Relay 地址）
	//NAT 后节点必须发布 Relay 地址才能被联系
	allAddrs := d.host.AdvertisedAddrs()

	// 2. 根据策略过滤地址
	var directAddrs, relayAddrs []string
	switch d.config.AddressPublishStrategy {
	case PublishRelayOnly:
		relayAddrs = filterRelayAddrs(allAddrs)
	case PublishDirectOnly:
		directAddrs = filterDirectAddrs(allAddrs)
	case PublishAll:
		directAddrs = filterDirectAddrs(allAddrs)
		relayAddrs = filterRelayAddrs(allAddrs)
	case PublishAuto:
		// 自动策略：根据可达性决定
		if d.config.ReachabilityProvider != nil {
			natType := d.config.ReachabilityProvider()
			if isPrivateNAT(natType) {
				// Private 节点只发布 relay 地址
				relayAddrs = filterRelayAddrs(allAddrs)
			} else {
				// Public 节点发布所有地址
				directAddrs = filterDirectAddrs(allAddrs)
				relayAddrs = filterRelayAddrs(allAddrs)
			}
		} else {
			// 默认发布所有地址
			directAddrs = filterDirectAddrs(allAddrs)
			relayAddrs = filterRelayAddrs(allAddrs)
		}
	}

	// 3. 获取 NAT 类型和可达性
	var natType types.NATType
	var reachability types.Reachability
	if d.config.ReachabilityProvider != nil {
		natType = d.config.ReachabilityProvider()
		if isPrivateNAT(natType) {
			reachability = types.ReachabilityPrivate
		} else {
			reachability = types.ReachabilityPublic
		}
	} else {
		natType = types.NATTypeUnknown
		reachability = types.ReachabilityUnknown
	}

	// 4. 构建能力列表
	capabilities := d.buildCapabilities()

	// 5. 创建签名记录
	ttl := d.config.PeerRecordTTL
	if ttl == 0 {
		ttl = DefaultPeerRecordTTL
	}

	signed, err := d.localRecordManager.CreateSignedRecord(
		directAddrs,
		relayAddrs,
		natType,
		reachability,
		capabilities,
		ttl,
	)
	if err != nil {
		return fmt.Errorf("failed to create signed record: %w", err)
	}

	// 6. 发布到 DHT
	//使用内部方法
	return d.publishPeerRecordInternal(ctx, signed)
}

// ============================================================================
//                              Realm 成员发现与地址查询
// ============================================================================

// ProvideRealmMembership 声明自己是 Realm 成员
//
// 发布 Provider Record 到 DHT，声明本节点是指定 Realm 的成员。
// Key: /dep2p/v2/realm/<H(RealmID)>/members
//
// 这是"先发布后发现"模式的核心，无需入口节点即可加入 Realm。
//
// 参数:
//   - ctx: 上下文
//   - realmID: Realm 标识符
//
// 返回:
//   - error: 发布失败时返回错误
func (d *DHT) ProvideRealmMembership(ctx context.Context, realmID types.RealmID) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	key := RealmMembersKey(realmID)
	if err := d.Provide(ctx, key, true); err != nil {
		return fmt.Errorf("failed to provide realm membership: %w", err)
	}

	logger.Info("Realm 成员声明成功",
		"realmID", realmID,
		"key", key)
	return nil
}

// FindRealmMembers 查找 Realm 成员列表
//
// 通过 DHT Provider 机制查找指定 Realm 的所有成员。
// Key: /dep2p/v2/realm/<H(RealmID)>/members
//
// 参数:
//   - ctx: 上下文
//   - realmID: Realm 标识符
//
// 返回:
//   - ch: 成员 PeerID 通道（异步返回）
//   - err: 查询失败时返回错误
func (d *DHT) FindRealmMembers(ctx context.Context, realmID types.RealmID) (<-chan types.PeerID, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	key := RealmMembersKey(realmID)
	providerCh, err := d.FindProviders(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to find realm members: %w", err)
	}

	// 转换 PeerInfo channel 为 PeerID channel
	peerIDCh := make(chan types.PeerID, 16)
	go func() {
		defer close(peerIDCh)
		for peerInfo := range providerCh {
			select {
			case <-ctx.Done():
				return
			case peerIDCh <- peerInfo.ID:
			}
		}
	}()

	return peerIDCh, nil
}

// PublishRealmPeerRecord 发布 Realm 成员地址
//
// 将签名的 PeerRecord 发布到 DHT，供其他 Realm 成员查询。
// Key: /dep2p/v2/realm/<H(RealmID)>/peer/<NodeID>
//
// 参数:
//   - ctx: 上下文
//   - realmID: Realm 标识符
//   - record: 签名的 PeerRecord（序列化字节）
//
// 返回:
//   - error: 发布失败时返回错误
func (d *DHT) PublishRealmPeerRecord(ctx context.Context, realmID types.RealmID, record []byte) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	if d.host == nil {
		return fmt.Errorf("host not initialized")
	}

	nodeID := types.NodeID(d.host.ID())
	key := RealmPeerKey(realmID, nodeID)

	if err := d.PutValue(ctx, key, record); err != nil {
		return fmt.Errorf("failed to publish realm peer record: %w", err)
	}

	logger.Info("Realm PeerRecord 发布成功",
		"realmID", realmID,
		"nodeID", nodeID,
		"key", key,
		"recordSize", len(record))

	return nil
}

// FindRealmPeerRecord 查询 Realm 成员地址
//
// 从 DHT 查询指定 Realm 成员的 PeerRecord。
// Key: /dep2p/v2/realm/<H(RealmID)>/peer/<NodeID>
//
// 参数:
//   - ctx: 上下文
//   - realmID: Realm 标识符
//   - nodeID: 目标节点 ID
//
// 返回:
//   - record: 签名的 PeerRecord（序列化字节）
//   - err: 未找到时返回 ErrNotFound
func (d *DHT) FindRealmPeerRecord(ctx context.Context, realmID types.RealmID, nodeID types.NodeID) ([]byte, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	key := RealmPeerKey(realmID, nodeID)
	record, err := d.GetValue(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to find realm peer record: %w", err)
	}

	return record, nil
}

// buildCapabilities 构建能力列表
func (d *DHT) buildCapabilities() []string {
	var caps []string

	// 检查是否支持 DHT 服务
	if d.started.Load() {
		caps = append(caps, "dht-server")
	}

	// 可以根据 host 的配置添加更多能力
	// 例如：relay, nat-traversal 等

	return caps
}

func (d *DHT) getPeerstoreAddrsBySource(peerID types.PeerID, sources ...corepeerstore.AddressSource) []types.Multiaddr {
	if d.peerstore == nil {
		return nil
	}

	sourceSet := make(map[corepeerstore.AddressSource]struct{}, len(sources))
	for _, source := range sources {
		sourceSet[source] = struct{}{}
	}

	if reader, ok := d.peerstore.(interface {
		AddrsWithSource(types.PeerID) []corepeerstore.AddressWithSource
	}); ok {
		addrsWithSource := reader.AddrsWithSource(peerID)
		if len(addrsWithSource) == 0 {
			return nil
		}
		var addrs []types.Multiaddr
		for _, entry := range addrsWithSource {
			if _, ok := sourceSet[entry.Source]; ok {
				addrs = append(addrs, entry.Addr)
			}
		}
		return addrs
	}

	return nil
}

func (d *DHT) hasPeerstoreSourceAccess() bool {
	if d.peerstore == nil {
		return false
	}
	_, ok := d.peerstore.(interface {
		AddrsWithSource(types.PeerID) []corepeerstore.AddressWithSource
	})
	return ok
}

func (d *DHT) getLocalRealmID() (types.RealmID, bool) {
	if d.localRecordManager == nil {
		return "", false
	}
	return d.localRecordManager.RealmID()
}

func (d *DHT) normalizeProviderKey(key string) string {
	if strings.HasPrefix(key, KeyPrefix+"/") {
		return key
	}
	if realmID, ok := d.getLocalRealmID(); ok {
		return RealmProviderKey(realmID, key)
	}
	return SystemKey("provider", []byte(key))
}

// LocalRecordManager 返回本地 PeerRecord 管理器（用于测试）
func (d *DHT) LocalRecordManager() *LocalPeerRecordManager {
	return d.localRecordManager
}

// UnpublishPeerRecord 取消发布 PeerRecord
//
// Phase D Step D3 对齐: 优雅关闭时调用，通知 DHT 网络本节点即将离线。
// 实现逻辑：
//  1. 清理 localRecordManager 状态，阻止后续续期
//  2. 从本地 peerRecordStore 删除记录
//  3. 可选：向最近的 K 个节点发送 TTL=0 的记录作为删除通知
//
// 参数:
//   - ctx: 上下文
//
// 返回:
//   - error: 取消发布失败时返回错误
func (d *DHT) UnpublishPeerRecord(_ context.Context) error {
	if d.localRecordManager == nil {
		return nil // 未初始化则无需清理
	}

	nodeID := d.localRecordManager.NodeID()
	realmID, hasRealm := d.localRecordManager.RealmID()

	logger.Info("开始取消发布 PeerRecord",
		"nodeID", nodeID,
		"hasRealm", hasRealm,
		"realmID", realmID)

	// 1. 清理 localRecordManager 状态
	// 这会阻止 republishLoop 继续发布
	d.localRecordManager.Clear()

	// 2. 从本地 peerRecordStore 删除记录
	if d.peerRecordStore != nil {
		// 删除全局 Key
		globalKey := GlobalPeerKey(nodeID)
		d.peerRecordStore.Delete(globalKey)
		logger.Debug("已删除全局 PeerRecord", "key", globalKey)

		// 如果有 Realm，也删除 Realm Key
		if hasRealm && realmID != "" {
			realmKey := RealmPeerKey(realmID, nodeID)
			d.peerRecordStore.Delete(realmKey)
			logger.Debug("已删除 Realm PeerRecord", "key", realmKey)
		}
	}

	// 3. 可选：发布 TTL=0 的删除通知（向最近的 K 个节点通知）
	// 这允许其他节点尽快知道本节点即将离线
	// 当前简化实现：不主动通知，依赖 TTL 过期自然清理
	// 未来可扩展为发送删除通知

	logger.Info("PeerRecord 取消发布完成", "nodeID", nodeID)
	return nil
}
