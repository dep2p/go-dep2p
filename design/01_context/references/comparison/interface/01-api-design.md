# 跨产品对比：API 设计

> **对比产品**: iroh、go-libp2p、torrent  
> **分析日期**: 2026-01-11

---

## 1. 概述

本文对比分析三个 P2P 产品的 API 设计，包括核心接口、使用模式、配置方式、错误处理等。

---

## 2. API 设计对比矩阵

| 特性 | iroh | go-libp2p | torrent |
|------|------|-----------|---------|
| **入口类型** | `Endpoint` | `Host` | `Client` |
| **配置方式** | Builder 模式 | 选项函数 | 配置结构体 |
| **异步模型** | async/await | sync + context | sync + channels |
| **错误处理** | Result | error | error |
| **资源管理** | RAII | 显式 Close | 显式 Close |
| **事件通知** | Watcher | Notifiee | Callbacks |

---

## 3. 入口 API 对比

### 3.1 iroh Endpoint

```rust
/// 核心入口点
pub struct Endpoint { /* ... */ }

impl Endpoint {
    /// 创建 Builder
    pub fn builder() -> Builder { ... }
    
    /// 本地 NodeID
    pub fn node_id(&self) -> NodeId { ... }
    
    /// 连接到远端
    pub async fn connect(&self, addr: NodeAddr, alpn: &[u8]) -> Result<Connection> { ... }
    
    /// 接受连接
    pub async fn accept(&self) -> Result<Connecting> { ... }
    
    /// 获取 NodeAddr (用于分享)
    pub async fn node_addr(&self) -> Result<NodeAddr> { ... }
    
    /// 添加发现服务
    pub fn add_discovery(&self, discovery: impl Discovery) { ... }
    
    /// 关闭
    pub async fn close(self) -> Result<()> { ... }
}
```

**使用示例**：

```rust
// 创建
let endpoint = Endpoint::builder()
    .secret_key(key)
    .alpns(vec![b"my-app".to_vec()])
    .relay_mode(RelayMode::Default)
    .bind()
    .await?;

// 服务端
loop {
    let connecting = endpoint.accept().await?;
    tokio::spawn(async move {
        let conn = connecting.await?;
        let (send, recv) = conn.accept_bi().await?;
        // 处理...
    });
}

// 客户端
let conn = endpoint.connect(addr, b"my-app").await?;
let (send, recv) = conn.open_bi().await?;
```

---

### 3.2 go-libp2p Host

```go
// 核心入口点
type Host interface {
    // 身份
    ID() peer.ID
    Addrs() []ma.Multiaddr
    
    // 连接
    Network() Network
    Connect(ctx context.Context, pi peer.AddrInfo) error
    
    // 流
    SetStreamHandler(pid protocol.ID, handler StreamHandler)
    NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (Stream, error)
    
    // 信息
    Peerstore() peerstore.Peerstore
    
    // 生命周期
    Close() error
}
```

**使用示例**：

```go
// 创建
host, _ := libp2p.New(
    libp2p.Identity(priv),
    libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
    libp2p.Security(noise.ID, noise.New),
    libp2p.Transport(tcp.NewTCPTransport),
)

// 服务端
host.SetStreamHandler("/my-app/1.0.0", func(s network.Stream) {
    defer s.Close()
    // 处理...
})

// 客户端
s, _ := host.NewStream(ctx, peerID, "/my-app/1.0.0")
defer s.Close()
// 使用...
```

---

### 3.3 torrent Client

```go
// 核心入口点
type Client struct { /* ... */ }

func NewClient(cfg *ClientConfig) (*Client, error) { ... }

func (cl *Client) AddTorrent(mi *metainfo.MetaInfo) (t *Torrent, new bool) { ... }
func (cl *Client) AddMagnet(uri string) (t *Torrent, err error) { ... }
func (cl *Client) AddTorrentFromFile(filename string) (t *Torrent, err error) { ... }

func (cl *Client) Torrents() []*Torrent { ... }
func (cl *Client) Stats() ClientStats { ... }
func (cl *Client) Close() { ... }
```

**使用示例**：

```go
// 创建
config := torrent.NewDefaultClientConfig()
config.DataDir = "/data"
client, _ := torrent.NewClient(config)

// 添加 torrent
t, _ := client.AddMagnet(magnetLink)
<-t.GotInfo()

// 下载
t.DownloadAll()
client.WaitAll()

// 流式读取
reader := t.NewReader()
io.Copy(dst, reader)
```

---

## 4. 配置方式对比

### 4.1 iroh Builder 模式

```rust
pub struct Builder {
    secret_key: Option<SecretKey>,
    alpns: Vec<Vec<u8>>,
    relay_mode: RelayMode,
    discovery: Vec<Box<dyn Discovery>>,
    // ...
}

impl Builder {
    pub fn secret_key(mut self, key: SecretKey) -> Self { ... }
    pub fn alpns(mut self, alpns: Vec<Vec<u8>>) -> Self { ... }
    pub fn relay_mode(mut self, mode: RelayMode) -> Self { ... }
    pub fn discovery(mut self, d: impl Discovery) -> Self { ... }
    
    pub async fn bind(self) -> Result<Endpoint> { ... }
}

// 使用
let endpoint = Endpoint::builder()
    .secret_key(key)
    .alpns(vec![b"app".to_vec()])
    .relay_mode(RelayMode::Default)
    .bind()
    .await?;
```

### 4.2 go-libp2p 选项函数

```go
type Option func(*Config) error

func Identity(sk crypto.PrivKey) Option { ... }
func ListenAddrs(addrs ...ma.Multiaddr) Option { ... }
func Security(id protocol.ID, constructor interface{}) Option { ... }
func Transport(constructor interface{}) Option { ... }

// 使用
host, _ := libp2p.New(
    libp2p.Identity(priv),
    libp2p.ListenAddrs(addr),
    libp2p.Security(noise.ID, noise.New),
)
```

### 4.3 torrent 配置结构体

```go
type ClientConfig struct {
    DataDir              string
    NoDHT                bool
    DhtStartingNodes     func() ([]dht.Addr, error)
    ListenHost           func(network string) string
    ListenPort           int
    DisableTCP           bool
    DisableUTP           bool
    // ...很多字段
}

// 使用
config := torrent.NewDefaultClientConfig()
config.DataDir = "/data"
config.ListenPort = 6881
client, _ := torrent.NewClient(config)
```

### 4.4 配置方式对比

| 方式 | 优点 | 缺点 | 使用产品 |
|------|------|------|----------|
| **Builder** | 链式调用、类型安全 | 样板代码 | iroh |
| **选项函数** | 灵活、可组合 | 错误处理复杂 | go-libp2p |
| **配置结构体** | 简单、直观 | 字段多时冗余 | torrent |

---

## 5. 连接与流 API 对比

### 5.1 连接 API

#### iroh Connection

```rust
pub struct Connection { /* ... */ }

impl Connection {
    pub fn remote_node_id(&self) -> NodeId { ... }
    pub fn alpn(&self) -> &[u8] { ... }
    
    pub async fn open_bi(&self) -> Result<(SendStream, RecvStream)> { ... }
    pub async fn open_uni(&self) -> Result<SendStream> { ... }
    pub async fn accept_bi(&self) -> Result<(SendStream, RecvStream)> { ... }
    pub async fn accept_uni(&self) -> Result<RecvStream> { ... }
    
    pub async fn close(&self, code: u32, reason: &[u8]) { ... }
}
```

#### go-libp2p Stream

```go
type Stream interface {
    io.Reader
    io.Writer
    io.Closer
    
    Protocol() protocol.ID
    Conn() Conn
    
    SetDeadline(time.Time) error
    SetReadDeadline(time.Time) error
    SetWriteDeadline(time.Time) error
    
    Reset() error
    CloseRead() error
    CloseWrite() error
}
```

### 5.2 对比表

| 操作 | iroh | go-libp2p | torrent |
|------|------|-----------|---------|
| **打开流** | `conn.open_bi()` | `host.NewStream()` | - |
| **接受流** | `conn.accept_bi()` | StreamHandler | - |
| **读取** | `recv.read()` | `stream.Read()` | `conn.Read()` |
| **写入** | `send.write()` | `stream.Write()` | `conn.Write()` |
| **关闭** | `stream.finish()` | `stream.Close()` | `conn.Close()` |
| **重置** | `stream.reset()` | `stream.Reset()` | - |

---

## 6. 事件与通知 API 对比

### 6.1 iroh Watcher

```rust
// 使用 n0_watcher 模式
pub struct Watcher<T> { /* ... */ }

impl<T: Clone> Watcher<T> {
    pub fn get(&self) -> T { ... }
    pub fn initialized(&self) -> impl Future<Output = T> { ... }
    pub async fn updated(&mut self) -> Option<T> { ... }
    pub fn stream(&self) -> impl Stream<Item = T> { ... }
}

// 使用示例
let mut watcher = endpoint.connection_watcher();
while let Some(event) = watcher.updated().await {
    match event {
        ConnectionEvent::Connected(id) => { ... }
        ConnectionEvent::Disconnected(id) => { ... }
    }
}
```

### 6.2 go-libp2p Notifiee

```go
type Notifiee interface {
    Listen(Network, ma.Multiaddr)
    ListenClose(Network, ma.Multiaddr)
    Connected(Network, Conn)
    Disconnected(Network, Conn)
    OpenedStream(Network, Stream)
    ClosedStream(Network, Stream)
}

// 使用示例
type MyNotifiee struct {}

func (n *MyNotifiee) Connected(net Network, conn Conn) {
    log.Printf("Connected to %s", conn.RemotePeer())
}

host.Network().Notify(&MyNotifiee{})
```

### 6.3 torrent Callbacks

```go
// 通过配置回调
config := torrent.NewDefaultClientConfig()
config.Callbacks.PeerConnClosed = func(t *Torrent, pc *PeerConn) {
    log.Printf("Peer disconnected: %s", pc.RemoteAddr)
}

// 使用 channel
t.SubscribePieceStateChanges()
```

### 6.4 对比表

| 特性 | iroh | go-libp2p | torrent |
|------|------|-----------|---------|
| **模式** | Watcher/Stream | 接口回调 | 回调/Channel |
| **异步** | ✅ async | 同步回调 | 同步回调 |
| **取消订阅** | Drop watcher | 不支持 | Close channel |
| **多订阅者** | 每个 clone | 多次 Notify | 多个 channel |

---

## 7. 错误处理对比

### 7.1 iroh 错误

```rust
// 使用 n0_error 宏
#[derive(Debug, Error)]
pub enum Error {
    #[error("connection failed: {0}")]
    ConnectionFailed(#[source] quinn::ConnectionError),
    
    #[error("stream error: {0}")]
    StreamError(#[source] quinn::WriteError),
    
    #[error("no relay available")]
    NoRelay,
}

// 使用
let conn = endpoint.connect(addr, alpn).await?;
```

### 7.2 go-libp2p 错误

```go
// 标准 error
var (
    ErrNoRemoteAddrs    = errors.New("no remote addresses")
    ErrNoGoodAddresses  = errors.New("no good addresses")
    ErrClosed           = errors.New("host is closed")
)

// 使用
if err := host.Connect(ctx, peerInfo); err != nil {
    if errors.Is(err, context.Canceled) {
        // 取消
    } else if errors.Is(err, swarm.ErrDialBackoff) {
        // 重试退避
    }
}
```

### 7.3 torrent 错误

```go
// 简单 error
var (
    ErrTorrentClosed  = errors.New("torrent closed")
    ErrNoPieces       = errors.New("no pieces")
)

// panic 用于内部错误
if condition {
    panic("unexpected state")
}
```

---

## 8. 高级 API 对比

### 8.1 发现 API

#### iroh

```rust
pub trait Discovery: Send + Sync + 'static {
    fn publish(&self, info: &EndpointData) -> BoxFuture<'_, Result<()>>;
    fn resolve(&self, id: EndpointId) -> BoxFuture<'_, Result<EndpointAddr>>;
}
```

#### go-libp2p

```go
type Discovery interface {
    Advertiser
    Discoverer
}

type Advertiser interface {
    Advertise(ctx context.Context, ns string, opts ...Option) (time.Duration, error)
}

type Discoverer interface {
    FindPeers(ctx context.Context, ns string, opts ...Option) (<-chan peer.AddrInfo, error)
}
```

### 8.2 路由 API

#### go-libp2p

```go
type Routing interface {
    ContentRouting
    PeerRouting
    ValueStore
}

type PeerRouting interface {
    FindPeer(ctx context.Context, id peer.ID) (peer.AddrInfo, error)
}
```

---

## 9. API 使用模式对比

### 9.1 简单使用

#### iroh

```rust
// 5 行代码启动节点
let endpoint = Endpoint::builder().bind().await?;
let addr = endpoint.node_addr().await?;
println!("Listening on: {:?}", addr);
let conn = endpoint.accept().await?.await?;
```

#### go-libp2p

```go
// 3 行代码启动节点
host, _ := libp2p.New()
defer host.Close()
fmt.Printf("ID: %s\n", host.ID())
```

#### torrent

```go
// 5 行代码下载
client, _ := torrent.NewClient(nil)
t, _ := client.AddMagnet(magnet)
<-t.GotInfo()
t.DownloadAll()
client.WaitAll()
```

### 9.2 复杂使用

| 场景 | iroh | go-libp2p | torrent |
|------|------|-----------|---------|
| **自定义传输** | Preset | Transport 选项 | 不支持 |
| **自定义发现** | discovery() | Discovery 选项 | 不支持 |
| **连接拦截** | Hooks | ConnGater | 不支持 |
| **资源限制** | 配置 | ResourceManager | Config |

---

## 10. 对 DeP2P 的启示

### 10.1 API 设计建议

| 决策 | 建议 | 参考 |
|------|------|------|
| **配置方式** | Builder + 选项函数 | iroh + libp2p |
| **入口设计** | 单一 Node 入口 | iroh Endpoint |
| **错误处理** | 类型化错误 | iroh |
| **事件通知** | Channel 模式 | Go 习惯 |
| **资源管理** | 显式 Close | Go 习惯 |

### 10.2 DeP2P API 设计

```go
// 入口
type Node struct { /* ... */ }

func NewNode(opts ...Option) (*Node, error) { ... }

// Builder 风格
type NodeBuilder struct { /* ... */ }

func Builder() *NodeBuilder { ... }
func (b *NodeBuilder) WithIdentity(key PrivateKey) *NodeBuilder { ... }
func (b *NodeBuilder) WithRelay(relay RelayConfig) *NodeBuilder { ... }
func (b *NodeBuilder) Build() (*Node, error) { ... }

// Node 方法
func (n *Node) ID() NodeID { ... }
func (n *Node) Connect(ctx context.Context, addr NodeAddr) (Connection, error) { ... }
func (n *Node) Accept(ctx context.Context) (Connection, error) { ... }
func (n *Node) Close() error { ... }

// Realm 相关
func (n *Node) JoinRealm(ctx context.Context, realm RealmID, psk []byte) error { ... }
func (n *Node) LeaveRealm(realm RealmID) error { ... }
func (n *Node) RealmPeers(realm RealmID) []NodeID { ... }

// 事件
func (n *Node) Events() <-chan Event { ... }
```

### 10.3 使用示例

```go
// 创建节点
node, _ := dep2p.Builder().
    WithIdentity(key).
    WithRelay(relayConfig).
    Build()
defer node.Close()

// 加入 Realm
node.JoinRealm(ctx, realmID, psk)

// 处理事件
go func() {
    for event := range node.Events() {
        switch e := event.(type) {
        case *PeerJoinedEvent:
            log.Printf("Peer joined: %s", e.PeerID)
        case *PeerLeftEvent:
            log.Printf("Peer left: %s", e.PeerID)
        }
    }
}()

// 连接
conn, _ := node.Connect(ctx, peerAddr)
stream, _ := conn.OpenStream(ctx)
```

---

## 11. 总结

| 产品 | API 特点 | 评价 |
|------|----------|------|
| **iroh** | 简洁、现代、async | 优雅 |
| **go-libp2p** | 灵活、模块化 | 成熟 |
| **torrent** | 专注、简单 | 实用 |

DeP2P 应该：
1. 结合 iroh 的简洁和 libp2p 的灵活
2. 采用 Builder + 选项函数配置
3. 使用 Channel 进行事件通知
4. 设计 Realm 感知的 API

---

**分析日期**：2026-01-11
