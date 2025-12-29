# Relay Client 中继客户端实现

## 概述

中继客户端实现，用于通过中继服务器与其他节点建立连接。

## 文件结构

```
client/
├── README.md        # 本文件
└── client.go        # 客户端实现
```

## 核心实现

### client.go

```go
// Client 中继客户端
type Client struct {
    transport    transportif.Transport
    identity     identityif.Identity
    relays       map[types.NodeID]*relayInfo
    reservations map[types.NodeID]*Reservation
    mu           sync.RWMutex
}

// NewClient 创建中继客户端
func NewClient(transport transportif.Transport, identity identityif.Identity) *Client

// Reserve 预留资源
func (c *Client) Reserve(ctx context.Context, relay types.NodeID) (relayif.Reservation, error)

// Connect 通过中继连接
func (c *Client) Connect(ctx context.Context, relay, dest types.NodeID) (transportif.Conn, error)

// FindRelays 发现中继服务器
func (c *Client) FindRelays(ctx context.Context) ([]types.NodeID, error)
```

## 预留流程

```
Client                          Relay Server
   |                                 |
   |-- RESERVE Request ------------->|
   |                                 |
   |<-- RESERVE Response ------------|
   |    (relay addresses, ttl)       |
   |                                 |
   |== Periodically refresh =======>|
```

## 连接流程

```
Client A                 Relay Server                Client B
   |                          |                          |
   |-- CONNECT(dest=B) ------>|                          |
   |                          |-- Notify B ------------->|
   |                          |<-- Accept --------------|
   |<-- CONNECT OK ----------|                          |
   |                          |                          |
   |<=== Data forwarding ====>|<=== Data forwarding ===>|
```

## 自动中继

```go
// AutoRelay 自动中继管理
type AutoRelay struct {
    client       *Client
    enabled      bool
    numRelays    int
    reservations []relayif.Reservation
}

// 自动发现和预留中继
func (ar *AutoRelay) maintain(ctx context.Context) {
    // 监控当前中继状态
    // 自动发现新中继
    // 保持最小中继数
}
```

