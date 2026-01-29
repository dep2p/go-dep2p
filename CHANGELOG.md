# Changelog

所有重要变更都会记录在此文件中。

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

---

## [Unreleased]

暂无

---

## [v0.2.0-beta.1] - 2026-01-29

**代号：v2.0 架构重构**

完成 v2.0 核心架构重构：DHT 权威目录 + Relay 三大职责 + 仅 ID 连接，首个 Beta 测试版本。

### 核心架构（v2.0）

- **五层软件架构**
  - API → Protocol → Realm → Core ↔ Discovery
  - 依赖方向清晰，职责分离

- **DHT 权威目录模型**
  - DHT 是地址解析的权威来源
  - Relay 地址簿作为缓存加速层
  - 地址发现优先级：Peerstore → MemberList → DHT → Relay

- **Relay 三大职责**
  - 缓存加速层：维护连接成员的地址信息
  - 打洞协调信令：作为打洞协调的信令通道
  - 数据通信保底：直连和打洞都失败时确保可达

- **仅 ID 连接**
  - Realm 内支持纯 NodeID 连接，自动地址发现
  - 跨 Realm 必须提供地址（设计约束）

### NAT 穿透

- **NAT 三层能力**
  - 外部地址发现：STUN 查询 + 观察地址收集
  - 打洞：通过信令通道协调，在 NAT 上"打开洞"
  - 中继：直连/打洞失败时的兜底通信

- **智能连接策略**
  - 直连 → 打洞 → Relay 自动回退
  - 打洞成功后保留 Relay 连接作为备份

### RelayCircuit 多路复用

- **电路状态机**
  - Active → Stale → Closed 完整生命周期
  - EventBus 状态变更事件集成

- **心跳保活机制**
  - 防止空闲电路被错误关闭
  - Stale 状态可恢复为 Active

- **双向流支持**
  - 出站电路可接收入站流
  - 支持 PSK 认证流程

### 基础设施融合

- Bootstrap + Relay 融合部署（推荐模式）
- 基础设施节点跳过 Realm 认证
- 基础设施节点不参与 PubSub Mesh

### 协议集中化

- 统一使用 `pkg/protocol` 作为单一真相源
- 完成所有模块的协议迁移

---

## [v0.1.0-alpha] - 2026-01-11

首个 Alpha 预览版本，展示核心架构和基础功能。

### Added

- **身份优先架构**
  - Ed25519 密钥对生成
  - NodeID 派生（公钥哈希）
  - 身份持久化支持

- **多协议传输**
  - TCP 传输层支持
  - QUIC 传输层支持（实验性）
  - 连接管理器

- **Realm 隔离**
  - Realm 创建和管理
  - PSK（预共享密钥）认证
  - Realm 内成员发现

- **中继系统**
  - Relay 控制面
  - Relay 数据面
  - 中继连接和转发

---

[Unreleased]: https://github.com/dep2p/go-dep2p/compare/v0.2.0-beta.1...HEAD
[v0.2.0-beta.1]: https://github.com/dep2p/go-dep2p/compare/v0.1.0-alpha...v0.2.0-beta.1
[v0.1.0-alpha]: https://github.com/dep2p/go-dep2p/releases/tag/v0.1.0-alpha
