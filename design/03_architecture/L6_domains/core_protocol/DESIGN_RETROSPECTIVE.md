# Core Protocol 设计复盘

> **日期**: 2026-01-13  
> **版本**: v1.0.0  
> **状态**: ✅ 已完成

---

## 一、设计目标 vs 实现

### 1.1 功能需求

| 需求 | 目标 | 实际 | 状态 |
|------|------|------|------|
| 协议注册 | 管理协议 ID 与处理器映射 | ✅ Registry 实现 | ✅ |
| 协议路由 | 根据协议 ID 路由流 | ✅ Router 实现 | ✅ |
| 协议协商 | multistream-select 协商 | ✅ Negotiator 实现 | ✅ |
| Ping 协议 | 存活检测和 RTT 测量 | ✅ 完整实现 | ✅ |
| Identify 协议 | 节点身份信息交换 | ✅ 基础版（JSON）| ✅ |

### 1.2 测试覆盖

| 模块 | 目标 | 实际 | 状态 |
|------|------|------|------|
| 主包 | > 70% | 60.7% | ⚠️ |
| Ping | > 50% | 29.0% | ⚠️ |
| Identify | > 50% | 0% | ⚠️ |

**说明**: Negotiator 和系统协议需要完整的 Host 实现才能充分测试

---

## 二、与 go-libp2p 对比

### 2.1 采用的设计

| 特性 | go-libp2p | DeP2P | 说明 |
|------|-----------|-------|------|
| Protocol Router | ✅ | ✅ | 相同机制 |
| Protocol Switch | ✅ | ✅ (分离) | Registry + Router |
| multistream-select | ✅ | ✅ | 使用相同库 |
| Ping 协议 | ✅ | ✅ | 32 字节回显 |
| Identify 协议 | ✅ Protobuf | ✅ JSON | v1.0 简化 |

### 2.2 简化的设计

| 特性 | go-libp2p | DeP2P | 原因 |
|------|-----------|-------|------|
| Protobuf 编码 | ✅ | ❌ JSON | v1.0 简化，v1.1+ 迁移 |
| Identify Push | ✅ | ❌ | v1.1+ 实现 |
| AutoNAT | ✅ v2 | ❌ | v1.1+ 实现 |
| HolePunch | ✅ | ❌ | v1.1+ 实现 |
| Relay | ✅ v2 | ❌ | v1.1+ 实现 |

### 2.3 代码对比

**代码量**:
- go-libp2p protocols: ~5000+ 行
- DeP2P v1.0: ~600 行

**复杂度**:
- go-libp2p: 高（完整实现）
- DeP2P v1.0: 低（核心功能）

---

## 三、架构决策记录 (ADR)

### ADR-1: Registry 与 Router 分离

**决策**: 分离注册和路由逻辑

**理由**:
- Registry 专注于存储管理
- Router 专注于流路由
- 单一职责原则

**权衡**:
- ✅ 易于测试
- ✅ 职责清晰
- ⚠️ 多一层抽象

### ADR-2: Identify 使用 JSON 编码

**决策**: v1.0 使用 JSON，v1.1+ 迁移 Protobuf

**理由**:
- JSON 实现简单
- 不需要 protoc 编译
- 降低初期复杂度

**权衡**:
- ✅ 实现简单
- ✅ 易于调试
- ⚠️ 与 libp2p 不兼容（可后续迁移）
- ⚠️ 编码效率较低

### ADR-3: v1.0 仅实现 Ping 和 Identify

**决策**: 暂不实现 AutoNAT、HolePunch、Relay

**理由**:
- Ping 和 Identify 是最核心的协议
- 其他协议依赖更多模块（NAT、Relay 等）
- 降低初期复杂度

**权衡**:
- ✅ 快速上线
- ✅ 专注核心
- ⚠️ 功能不完整（可后续扩展）

---

## 四、实现亮点

### 4.1 清晰的分层设计

```
Router (路由器)
  ├── Registry (注册表)
  └── Negotiator (协商器)
      └── go-multistream (协议协商)

系统协议:
  ├── Ping (32 字节回显)
  └── Identify (JSON 信息交换)
```

### 4.2 可扩展架构

- ✅ 易于添加新协议
- ✅ 支持模式匹配（通配符）
- ✅ 支持 Realm 和应用协议

### 4.3 标准协议协商

- ✅ 使用 multistream-select
- ✅ 与 libp2p 兼容
- ✅ 支持客户端和服务器模式

---

## 五、已知限制

### 5.1 覆盖率限制

- 主包 60.7%（需要 Connection mock）
- Ping 29.0%（需要 Host 实现）
- Identify 0%（需要完整集成）

**原因**: Negotiator 和系统协议依赖 Host，在 Host 实现后可提升覆盖率

### 5.2 功能限制

- Identify 使用 JSON（非 Protobuf）
- 不支持 Identify Push
- 暂无 AutoNAT、HolePunch、Relay

### 5.3 依赖限制

- 与 core_host 存在循环依赖（通过最小接口解耦）
- Negotiator 需要 Connection 支持 io.Reader/Writer

---

## 六、改进建议

### 6.1 短期改进（v1.1）

- [ ] 提升测试覆盖率至 70%+
- [ ] Identify 迁移至 Protobuf
- [ ] 实现 Identify Push
- [ ] 添加性能基准测试

### 6.2 长期改进（v2.0）

- [ ] 实现 AutoNAT v2
- [ ] 实现 HolePunch
- [ ] 实现 Circuit Relay v2
- [ ] 添加 Metrics 指标

---

## 七、经验教训

### 7.1 成功经验

✅ **复用设计**: multistream-select 协商逻辑与 core_upgrader 一致

✅ **简化优先**: JSON 编码降低初期复杂度

✅ **清晰分层**: Registry、Router、Negotiator 职责分离

### 7.2 改进空间

⚠️ **测试覆盖**: 需要更完整的 mock 才能充分测试

⚠️ **Host 依赖**: 系统协议依赖 Host，需要解耦

---

## 八、总结

### 8.1 目标达成

✅ **协议框架完整**: Registry + Router + Negotiator  
✅ **系统协议实现**: Ping + Identify (基础版)  
✅ **文档完善**: 6 个文档文件  
⚠️ **测试覆盖**: 主包 60.7%（待提升）

### 8.2 核心成就

1. **协议注册表** - 完整实现，支持模式匹配
2. **Ping 协议** - 真实回显，RTT 测量
3. **Identify 协议** - 节点信息交换（JSON）
4. **multistream 集成** - 标准协议协商

### 8.3 后续演进

**v1.0** (当前): 核心框架 + Ping + Identify (JSON)  
**v1.1** (计划): Protobuf + Identify Push  
**v2.0** (未来): AutoNAT + HolePunch + Relay

---

**复盘完成日期**: 2026-01-13  
**下一步**: 提升测试覆盖率 或 C3-04 core_swarm
