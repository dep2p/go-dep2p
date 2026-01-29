# Discovery 层测试用例

> Discovery 层模块的测试用例集

---

## 概述

本目录包含 Discovery 层 (`internal/discovery/`) 的测试用例，涵盖 DHT、mDNS、Bootstrap、DNS、Rendezvous 等发现机制。

---

## 代码位置

```
internal/discovery/
├── dht/                      # DHT 发现
│   ├── dht.go
│   ├── dht_test.go
│   ├── query.go
│   └── errors.go
├── mdns/                     # mDNS 局域网发现
│   ├── mdns.go
│   ├── mdns_test.go
│   └── integration_test.go
├── bootstrap/                # 引导节点
│   ├── bootstrap.go
│   ├── bootstrap_test.go
│   └── integration_test.go
├── dns/                      # DNS 发现
│   ├── dns.go
│   ├── dns_test.go
│   └── resolver.go
├── rendezvous/               # 汇合点
│   ├── rendezvous.go
│   ├── rendezvous_test.go
│   ├── point.go
│   └── store.go
└── coordinator/              # 发现协调器
    ├── coordinator.go
    ├── coordinator_test.go
    └── strategy.go
```

---

## DHT 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-DHT-0001 | DHT 初始化 | 单元 | P0 | ✅ |
| TST-DHT-0002 | 节点加入 DHT | 集成 | P0 | ✅ |
| TST-DHT-0003 | FindPeer 查询 | 单元 | P0 | ✅ |
| TST-DHT-0004 | Provider 发布 | 单元 | P1 | ✅ |
| TST-DHT-0005 | Provider 查询 | 单元 | P1 | ✅ |
| TST-DHT-0006 | 路由表更新 | 单元 | P1 | ✅ |
| TST-DHT-0007 | 节点发现 | 集成 | P0 | ✅ |

### 用例详情

#### TST-DHT-0003: FindPeer 查询

| 字段 | 值 |
|------|-----|
| **ID** | TST-DHT-0003 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **代码位置** | `internal/discovery/dht/dht_test.go` |

**测试目标**：验证 DHT 查找节点功能

---

## mDNS 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-MDNS-0001 | mDNS 初始化 | 单元 | P0 | ✅ |
| TST-MDNS-0002 | 服务广播 | 单元 | P0 | ✅ |
| TST-MDNS-0003 | 服务发现 | 单元 | P0 | ✅ |
| TST-MDNS-0004 | 局域网发现 | 集成 | P0 | ✅ |
| TST-MDNS-0005 | 多播响应 | 单元 | P1 | ✅ |

### 用例详情

#### TST-MDNS-0004: 局域网发现

| 字段 | 值 |
|------|-----|
| **ID** | TST-MDNS-0004 |
| **类型** | 集成测试 |
| **优先级** | P0 |
| **代码位置** | `internal/discovery/mdns/integration_test.go` |

**测试目标**：验证局域网内节点发现

---

## Bootstrap 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-BOOTSTRAP-0001 | Bootstrap 初始化 | 单元 | P0 | ✅ |
| TST-BOOTSTRAP-0002 | 引导节点连接 | 集成 | P0 | ✅ |
| TST-BOOTSTRAP-0003 | 重连策略 | 单元 | P1 | ✅ |
| TST-BOOTSTRAP-0004 | 节点列表管理 | 单元 | P1 | ✅ |

### 用例详情

#### TST-BOOTSTRAP-0002: 引导节点连接

| 字段 | 值 |
|------|-----|
| **ID** | TST-BOOTSTRAP-0002 |
| **类型** | 集成测试 |
| **优先级** | P0 |
| **代码位置** | `internal/discovery/bootstrap/integration_test.go` |

**测试目标**：验证连接到引导节点功能

---

## DNS 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-DNS-0001 | DNS 解析 | 单元 | P0 | ✅ |
| TST-DNS-0002 | TXT 记录解析 | 单元 | P1 | ✅ |
| TST-DNS-0003 | 缓存机制 | 单元 | P2 | ✅ |

---

## Rendezvous 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-RENDEZVOUS-0001 | Rendezvous 初始化 | 单元 | P0 | ✅ |
| TST-RENDEZVOUS-0002 | 命名空间注册 | 单元 | P0 | ✅ |
| TST-RENDEZVOUS-0003 | 节点发现 | 单元 | P0 | ✅ |
| TST-RENDEZVOUS-0004 | 注册过期 | 单元 | P1 | ✅ |
| TST-RENDEZVOUS-0005 | 持久化存储 | 单元 | P2 | ✅ |

---

## Coordinator 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-COORD-0001 | 协调器初始化 | 单元 | P0 | ✅ |
| TST-COORD-0002 | 策略选择 | 单元 | P1 | ✅ |
| TST-COORD-0003 | 多源协调 | 集成 | P1 | ✅ |

---

## 发现策略测试

| 场景 | 测试内容 | 预期行为 |
|------|----------|----------|
| 局域网优先 | mDNS 在局域网内优先 | 快速发现本地节点 |
| DHT 回退 | mDNS 失败后使用 DHT | 自动切换 |
| Bootstrap 启动 | 无已知节点时连接引导节点 | 成功启动网络 |
| 多源合并 | 多个发现源的结果合并 | 去重、排序 |

---

## 性能测试指标

| 模块 | 指标 | 目标 |
|------|------|------|
| mDNS | 局域网发现延迟 | ≤ 100ms |
| DHT | FindPeer 延迟 | ≤ 500ms |
| Bootstrap | 启动连接时间 | ≤ 2s |
| Coordinator | 策略切换时间 | ≤ 50ms |

---

## 代码覆盖目标

| 文件 | 目标覆盖率 | 当前状态 |
|------|-----------|----------|
| `dht/dht.go` | ≥ 80% | ✅ |
| `mdns/mdns.go` | ≥ 80% | ✅ |
| `bootstrap/bootstrap.go` | ≥ 80% | ✅ |
| `dns/dns.go` | ≥ 70% | ✅ |
| `rendezvous/rendezvous.go` | ≥ 80% | ✅ |
| `coordinator/coordinator.go` | ≥ 80% | ✅ |

---

**最后更新**：2026-01-15
