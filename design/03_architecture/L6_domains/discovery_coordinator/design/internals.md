# core_discovery 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/discovery/
├── module.go           # Fx 模块
├── service.go          # 组合发现服务
├── dht.go              # DHT 实现
├── mdns.go             # mDNS 实现
├── rendezvous.go       # Rendezvous 实现
└── discovery_test.go   # 测试
```

---

## DHT 实现

### K-Bucket

```
type KBucket struct {
    peers    []types.NodeID
    maxSize  int  // K = 20
}

// XOR 距离决定 bucket
func bucketIndex(local, remote types.NodeID) int {
    distance := xor(local, remote)
    return leadingZeros(distance)
}
```

### 查找算法

```
func (d *DHT) FindPeer(ctx context.Context, id types.NodeID) (PeerInfo, error) {
    // 1. 从路由表找最近的 α 个节点
    closest := d.routingTable.NearestPeers(id, alpha)
    
    // 2. 并行查询
    for _, peer := range closest {
        go d.queryPeer(ctx, peer, id, results)
    }
    
    // 3. 迭代查询直到找到或无更近节点
    for result := range results {
        if result.ID == id {
            return result, nil
        }
        // 继续查询更近的节点
    }
    
    return PeerInfo{}, ErrNotFound
}
```

---

## mDNS 实现

### 服务注册

```
const mdnsServiceTag = "_dep2p._udp"

func (m *MDNS) Start() error {
    // 注册 mDNS 服务
    service, err := mdns.Register(
        m.localID.String(),
        mdnsServiceTag,
        "",
        m.port,
        []string{"dep2p"},
        nil,
    )
    
    // 启动监听
    go m.listen()
    
    return nil
}
```

### 发现监听

```
func (m *MDNS) listen() {
    entries := make(chan *mdns.ServiceEntry)
    go mdns.Lookup(mdnsServiceTag, entries)
    
    for entry := range entries {
        peerID, _ := parseNodeID(entry.Name)
        addr := fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", 
            entry.AddrV4, entry.Port)
        
        m.notifee.HandlePeerFound(PeerInfo{
            ID:    peerID,
            Addrs: []multiaddr.Multiaddr{ma},
        })
    }
}
```

---

## Rendezvous 实现

### 注册流程

```
func (r *Rendezvous) Register(ctx context.Context, ns string, ttl time.Duration) error {
    // 找到 Rendezvous 服务节点
    server := r.getRendezvousServer()
    
    // 发送注册请求
    stream, _ := r.host.NewStream(ctx, server, rendezvousProto)
    
    msg := &pb.Register{
        Namespace: ns,
        TTL:       int64(ttl.Seconds()),
        Peer:      r.localInfo(),
    }
    
    return writeMsg(stream, msg)
}
```

### 发现流程

```
func (r *Rendezvous) Discover(ctx context.Context, ns string, limit int) ([]PeerInfo, error) {
    server := r.getRendezvousServer()
    stream, _ := r.host.NewStream(ctx, server, rendezvousProto)
    
    msg := &pb.Discover{
        Namespace: ns,
        Limit:     int32(limit),
    }
    writeMsg(stream, msg)
    
    resp := &pb.DiscoverResponse{}
    readMsg(stream, resp)
    
    return parsePeers(resp.Peers), nil
}
```

---

## 错误处理

```
var (
    ErrNotFound        = errors.New("discovery: peer not found")
    ErrNoRendezvous    = errors.New("discovery: no rendezvous server")
    ErrNamespaceEmpty  = errors.New("discovery: namespace cannot be empty")
)
```

---

**最后更新**：2026-01-11
