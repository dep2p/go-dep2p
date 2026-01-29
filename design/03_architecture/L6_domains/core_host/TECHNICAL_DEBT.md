# core_host 技术债管理

> **日期**: 2026-01-14  
> **版本**: v1.0.0

---

## 技术债清单

### TD-HOST-001: 高级地址观测（ObservedAddrsManager）

**当前状态**: v1.0 仅支持监听地址，使用简单的 AddrsFactory

**问题描述**:
- v1.0 的地址管理器仅返回监听地址
- 缺少外部观测地址收集机制
- 缺少地址验证和评分逻辑
- 缺少地址签名（SignedPeerRecord）

**阻塞原因**:
- 需要实现复杂的地址验证逻辑
- 需要与 Identify 协议深度集成
- 需要地址评分算法

**优先级**: P2

**预估工作量**: 2-3天

**解除阻塞条件**:
1. 实现 ObservedAddrsManager
2. 集成 Identify 协议的地址交换
3. 实现地址验证和评分
4. 实现地址签名和验证

**相关代码位置**:
- `internal/core/host/addrs.go`
- `internal/core/protocol/system/identify/`

**参考实现**:
- go-libp2p: `p2p/host/basic/addrs_manager.go`

---

### TD-HOST-002: AutoNAT v2 集成

**当前状态**: v1.0 使用 core_nat 的 AutoNAT 客户端

**问题描述**:
- 缺少 AutoNAT v2 服务端支持
- 缺少地址可达性检测完整流程

**阻塞原因**:
- 依赖 core_nat TD-002（AutoNAT 服务端）
- 需要与地址管理器集成

**优先级**: P2

**预估工作量**: 1-2天

**解除阻塞条件**:
1. core_nat TD-002 完成
2. 集成 AutoNAT v2 到 Host
3. 更新地址可达性状态

**相关代码位置**:
- `internal/core/host/lifecycle.go`
- `internal/core/nat/autonat.go`

**参考实现**:
- go-libp2p: `p2p/protocol/autonatv2/`

---

### TD-HOST-003: 地址过滤策略增强

**当前状态**: v1.0 使用简单的 AddrsFactory 函数

**问题描述**:
- 缺少公网/私网地址过滤
- 缺少 localhost 过滤
- 缺少地址优先级排序

**阻塞原因**:
- 需要更多实际使用场景
- 需要地址分类逻辑

**优先级**: P3

**预估工作量**: 1天

**解除阻塞条件**:
1. 实现地址分类（公网、私网、localhost）
2. 实现地址过滤器集合
3. 实现地址优先级排序

**相关代码位置**:
- `internal/core/host/addrs.go`

**参考实现**:
- go-libp2p: `core/network/multiaddr.go`

---

## 技术债统计

| ID | 名称 | 优先级 | 预估工作量 | 状态 |
|----|------|--------|-----------|------|
| TD-HOST-001 | ObservedAddrsManager | P2 | 2-3天 | 待实施 |
| TD-HOST-002 | AutoNAT v2 集成 | P2 | 1-2天 | 阻塞中 |
| TD-HOST-003 | 地址过滤策略 | P3 | 1天 | 待实施 |

**总计**: 3个技术债，4-6天工作量

---

## v1.1 实施建议

**优先顺序**:
1. TD-HOST-001（P2）- 地址观测是核心功能
2. TD-HOST-002（P2）- 等待 core_nat TD-002
3. TD-HOST-003（P3）- 根据实际需求决定

**依赖关系**:
- TD-HOST-002 依赖 core_nat TD-002
- TD-HOST-001 和 TD-HOST-003 可并行实施

---

**最后更新**: 2026-01-14
