# 区块链场景 P2P 需求

> **来源**：go-ethereum、Fabric、btcd 分析  
> **目标**：提炼区块链场景对 DeP2P 的需求

---

## 1. 场景概述

### 区块链 P2P 网络特点

| 特点 | 说明 |
|------|------|
| **消息密集** | 区块、交易持续广播 |
| **实时性要求** | 区块传播延迟影响共识 |
| **大消息** | 区块大小可达 MB 级别 |
| **网络隔离** | 主网/测试网需分离 |
| **节点众多** | 公链可达数万节点 |

### DeP2P 目标场景

```
DeP2P 区块链场景：

  1. 区块链 SDK 底层通信
     - 节点间区块/交易传播
     - 共识消息传递
     
  2. 联盟链节点网络
     - 类似 Fabric Channel 的隔离
     - 权限控制
     
  3. 私有链/测试网
     - 独立的网络隔离
     - 快速部署
```

---

## 2. 功能需求

### REQ-BC-001: 区块广播

| 属性 | 说明 |
|------|------|
| **优先级** | 高 |
| **来源** | go-ethereum, Fabric, btcd |
| **描述** | 新区块需要快速传播到全网 |

**需求详情**：

- 支持 Gossip 式广播（PubSub）
- 支持"先通告后请求"模式（inv → getdata）
- 支持区块哈希去重
- 延迟目标：< 1 秒到达 90% 节点

**DeP2P 映射**：

```
PubSub.Join("blocks")
PubSub.Publish(blockData)
PubSub.Subscribe() → 接收区块
```

---

### REQ-BC-002: 交易广播

| 属性 | 说明 |
|------|------|
| **优先级** | 高 |
| **来源** | go-ethereum, btcd |
| **描述** | 未确认交易需要传播到矿工/验证者 |

**需求详情**：

- 支持 Gossip 广播
- 支持交易批量发送
- 支持交易去重（避免重复处理）
- 支持交易优先级（高 gas 优先）

**DeP2P 映射**：

```
PubSub.Join("transactions")
PubSub.Publish(txData)
```

---

### REQ-BC-003: 区块/状态请求

| 属性 | 说明 |
|------|------|
| **优先级** | 高 |
| **来源** | go-ethereum, Fabric, btcd |
| **描述** | 节点需要请求特定区块或状态数据 |

**需求详情**：

- 支持请求-响应模式
- 支持批量请求（区块范围）
- 支持流式传输（大数据量）
- 支持超时和重试

**DeP2P 映射**：

```
Messaging.Request(peer, GetBlockRequest)
Streams.Open(peer, "sync") → 流式同步
```

---

### REQ-BC-004: 节点发现

| 属性 | 说明 |
|------|------|
| **优先级** | 高 |
| **来源** | go-ethereum, Fabric, btcd |
| **描述** | 自动发现网络中的其他节点 |

**需求详情**：

- 支持 Bootstrap 节点
- 支持 DHT 发现（Kademlia）
- 支持 Realm 内发现
- 支持节点地址交换

**DeP2P 映射**：

```
Bootstrap → 初始节点
DHT → 持续发现
Realm.Discovery() → 域内发现
```

---

### REQ-BC-005: 网络隔离

| 属性 | 说明 |
|------|------|
| **优先级** | 高 |
| **来源** | Fabric (Channel), go-ethereum (NetworkID) |
| **描述** | 不同网络的节点完全隔离 |

**需求详情**：

- 主网和测试网隔离
- 不同业务链隔离
- 消息不跨域传播
- 中继不跨域转发

**DeP2P 映射**：

```
node.JoinRealmWithKey("mainnet", mainnetPSK)
node.JoinRealmWithKey("testnet", testnetPSK)

// Realm 内通信
realm.Messaging().Send(peer, data)
realm.PubSub().Publish(topic, data)
```

---

### REQ-BC-006: 成员准入

| 属性 | 说明 |
|------|------|
| **优先级** | 中 |
| **来源** | Fabric (MSP) |
| **描述** | 控制谁能加入网络 |

**需求详情**：

- 联盟链需要准入控制
- 验证节点身份
- 支持动态加入/退出

**DeP2P 映射**：

```
// PSK 认证
node.JoinRealmWithKey("consortium", consortiumPSK)

// 成员验证
realm.IsMember(nodeID)
```

---

### REQ-BC-007: 共识消息

| 属性 | 说明 |
|------|------|
| **优先级** | 中 |
| **来源** | Fabric (选举), go-ethereum (共识) |
| **描述** | 共识协议消息需要可靠传递 |

**需求详情**：

- 点对点可靠传递
- 低延迟
- 消息签名验证

**DeP2P 映射**：

```
Messaging.Send(validator, voteData)
Messaging.Request(leader, proposalRequest)
```

---

## 3. 非功能需求

### REQ-BC-NF-001: 低延迟

| 属性 | 说明 |
|------|------|
| **优先级** | 高 |
| **指标** | 区块传播 < 1s，交易传播 < 500ms |

**说明**：

- 区块传播延迟影响共识效率
- 交易传播延迟影响用户体验
- QUIC 0-RTT 有助于降低延迟

---

### REQ-BC-NF-002: 高吞吐

| 属性 | 说明 |
|------|------|
| **优先级** | 高 |
| **指标** | 支持 1000+ TPS 的交易广播 |

**说明**：

- 高 TPS 场景下消息量大
- 需要高效的广播机制
- 需要批量消息支持

---

### REQ-BC-NF-003: 大消息支持

| 属性 | 说明 |
|------|------|
| **优先级** | 中 |
| **指标** | 支持 4MB+ 区块传输 |

**说明**：

- 区块大小可达 MB 级别
- 需要分片传输
- 需要流式传输支持

---

### REQ-BC-NF-004: NAT 友好

| 属性 | 说明 |
|------|------|
| **优先级** | 中 |
| **指标** | 支持家用网络环境 |

**说明**：

- 许多节点在 NAT 后面
- 需要 Relay 备选
- 需要打洞支持

---

## 4. DeP2P 特性映射

### 消息服务映射

| 区块链需求 | DeP2P 服务 | 协议 |
|------------|-----------|------|
| 区块广播 | PubSub | `/dep2p/app/{realm}/blocks/1.0.0` |
| 交易广播 | PubSub | `/dep2p/app/{realm}/txs/1.0.0` |
| 区块请求 | Messaging | `/dep2p/app/{realm}/sync/1.0.0` |
| 状态同步 | Streams | `/dep2p/app/{realm}/state/1.0.0` |
| 共识消息 | Messaging | `/dep2p/app/{realm}/consensus/1.0.0` |

### 隔离映射

| 区块链概念 | DeP2P 映射 |
|------------|-----------|
| 主网 | `realm-mainnet` |
| 测试网 | `realm-testnet` |
| 联盟链 Channel | `realm-{channel}` |
| 分片 | `realm-shard-{id}` |

### 角色映射

| 区块链角色 | DeP2P 行为 |
|------------|-----------|
| 全节点 | 订阅所有 Topic，处理所有消息 |
| 轻节点 | 仅订阅区块头 Topic |
| 验证者 | 额外订阅共识 Topic |
| 中继节点 | 运行 Relay |

---

## 5. 使用示例

### 区块链节点启动

```
启动流程：

  1. 启动 Node
     node = StartNode(opts)
     
  2. 加入 Realm (网络隔离)
     realm = node.JoinRealmWithKey("mainnet", mainnetPSK)
     
  3. 订阅区块
     blocks = realm.PubSub().Join("blocks")
     blocks.Subscribe()
     
  4. 订阅交易
     txs = realm.PubSub().Join("transactions")
     txs.Subscribe()
     
  5. 开始同步
     peers = realm.Discovery().FindPeers()
     for peer := range peers {
         stream = realm.Streams().Open(peer, "sync")
         // 同步区块...
     }
```

### 区块广播

```
广播新区块：

  // 验证者产生区块
  block = createBlock()
  
  // 广播到 Realm
  topic = realm.PubSub().Join("blocks")
  topic.Publish(block.Serialize())
```

### 请求区块

```
请求特定区块：

  request = BlockRequest{
      StartHeight: 1000,
      Count:       100,
  }
  
  response = realm.Messaging().Request(peer, request)
  blocks = response.Blocks
```

---

## 6. 需求优先级

### P0 (必须)

| 需求 | 说明 |
|------|------|
| REQ-BC-001 | 区块广播 |
| REQ-BC-003 | 区块/状态请求 |
| REQ-BC-004 | 节点发现 |
| REQ-BC-005 | 网络隔离 |

### P1 (重要)

| 需求 | 说明 |
|------|------|
| REQ-BC-002 | 交易广播 |
| REQ-BC-006 | 成员准入 |
| REQ-BC-NF-001 | 低延迟 |
| REQ-BC-NF-002 | 高吞吐 |

### P2 (增强)

| 需求 | 说明 |
|------|------|
| REQ-BC-007 | 共识消息 |
| REQ-BC-NF-003 | 大消息支持 |
| REQ-BC-NF-004 | NAT 友好 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [blockchain-p2p.md](../comparison/blockchain-p2p.md) | 横向对比 |
| [ethereum.md](../individual/ethereum.md) | go-ethereum 分析 |
| [fabric.md](../individual/fabric.md) | Fabric 分析 |
| [bitcoin.md](../individual/bitcoin.md) | btcd 分析 |

---

**最后更新**：2026-01-11
