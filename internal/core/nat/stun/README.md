# STUN 客户端实现

## 概述

基于 `pion/stun` 库的 STUN 客户端实现，用于外部地址发现和 NAT 类型检测。

## 文件结构

```
stun/
├── README.md        # 本文件
└── client.go        # STUN 客户端实现
```

## 核心实现

### client.go

```go
// Client STUN 客户端
type Client struct {
    servers []string
    timeout time.Duration
}

// NewClient 创建 STUN 客户端
func NewClient(servers []string, timeout time.Duration) *Client

// GetMappedAddress 获取映射地址
func (c *Client) GetMappedAddress(ctx context.Context) (core.Address, error)

// GetNATType 检测 NAT 类型
func (c *Client) GetNATType(ctx context.Context) (types.NATType, error)
```

## NAT 类型检测算法

```
                    +--------+
                    | Start  |
                    +--------+
                         |
                         v
              +-------------------+
              | Test I: Basic     |
              | Binding Request   |
              +-------------------+
                         |
            +------------+------------+
            | Response   | No Response|
            v            v            v
       +--------+    +--------+   +--------+
       | Test II|    | UDP    |   | Test II|
       +--------+    | Blocked|   | (alt)  |
            |        +--------+   +--------+
            v                          |
    +---------------+                  v
    | Same IP:Port? |          +-------------+
    +---------------+          | Symmetric   |
      |Yes    |No              +-------------+
      v       v
  +------+ +----------+
  | Full | | Test III |
  | Cone | +----------+
  +------+      |
           +----+----+
           |Yes      |No
           v         v
      +---------+ +----------------+
      |Restricted| |Port Restricted|
      +---------+ +----------------+
```

## STUN 消息格式

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|0 0|     STUN Message Type     |         Message Length        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         Magic Cookie                          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
|                     Transaction ID (96 bits)                  |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

## 依赖

- `github.com/pion/stun` - STUN 协议实现

