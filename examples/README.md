# DeP2P 示例集合

欢迎使用 DeP2P！本目录包含一系列循序渐进的示例，帮助你快速掌握 P2P 网络编程。

## 快速开始

如果你是第一次接触 DeP2P，建议按以下顺序学习：

```
1️⃣ Realm 入门示例 (realm/)        → 理解 Node Facade、JoinRealm、强制隔离
2️⃣ Echo 示例 (echo/)             → 学习协议注册、流通信
3️⃣ Chat 示例 (chat/)             → 掌握 mDNS 发现 + Realm 聊天室隔离（局域网）
4️⃣ Chat Public 示例 (chat_public/) → 公网三节点聊天（DialBy 三语义实践）⭐
5️⃣ Relay 示例 (relay/)           → 理解 NAT 穿透 + Relay 兜底连接
```

## 环境要求

- **Go 版本**: 1.21 或更高
- **操作系统**: Linux, macOS, Windows
- **网络**: 某些示例需要局域网或互联网连接

### 验证环境

```bash
# 检查 Go 版本
go version

# 克隆并进入项目
cd examples/

# 测试编译
go build ./...
```

## 示例列表

### 1️⃣ 基础示例 - QuickStart

**位置**: `examples/basic/`  
**难度**: ⭐ 入门级  
**学习时间**: 5 分钟

**你将学到**:
- 如何创建一个 P2P 节点
- 节点 ID 是什么
- 如何监听连接
- 如何注册协议处理器

**运行方式**:
```bash
# 从仓库根目录运行
go run ./examples/basic
```

**适合人群**: 完全没有 P2P 经验的新手

---

### 2️⃣ Echo 示例 - 消息回显

**位置**: `examples/echo/`  
**难度**: ⭐⭐ 初级  
**学习时间**: 15 分钟

**你将学到**:
- 如何建立点对点连接
- 什么是流（Stream）
- 如何发送和接收数据
- 监听模式 vs 拨号模式

**运行方式**:
```bash
cd examples/echo/

# 终端 1: 启动监听节点
go run main.go -mode listener

# 终端 2: 连接并发送消息
go run main.go -mode dialer -fulladdr <FullAddress>
```

**适合人群**: 理解基础概念后，想学习实际通信的开发者

**详细文档**: [echo/README.md](echo/README.md)

---

### 3️⃣ Chat 示例 - 局域网聊天

**位置**: `examples/chat/`  
**难度**: ⭐⭐⭐ 中级  
**学习时间**: 20 分钟

**你将学到**:
- 什么是 mDNS 自动发现
- 如何在局域网内发现其他节点
- 多对等方通信
- 交互式命令行程序

**运行方式**:
```bash
cd examples/chat/

# 终端 1
go run main.go -nick Alice

# 终端 2（同一局域网）
go run main.go -nick Bob
```

两个节点会自动发现并连接，然后就可以聊天了！

**适合人群**: 想构建实用 P2P 应用的开发者

**详细文档**: [chat/README.md](chat/README.md)

---

### 4️⃣ Chat Public 示例 - 公网聊天 ⭐

**位置**: `examples/chat_public/`  
**难度**: ⭐⭐⭐⭐ 高级  
**学习时间**: 30 分钟

**你将学到**:
- 什么是 Full Address 与 NodeID
- DialBy 三语义的实际应用
- ShareableAddrs 与 VerifiedDirect 约束
- 公网环境下的连接诊断

**运行方式（三节点）**:
```bash
# 第 1 步: VPS 上启动 Seed（公网可达）
go run main.go -mode seed -port 4001

# 第 2 步: 本地启动 Alice（复制 seed 输出的 Full Address）
go run main.go -mode peer -seed "<seedFullAddr>" -name alice

# 第 3 步: 另一台机器启动 Bob
go run main.go -mode peer -seed "<seedFullAddr>" -name bob

# 第 4 步: Alice 连接 Bob（按 NodeID）
/connect <Bob的NodeID>
```

**连接语义（DialBy 三语义）**:
- **冷启动**: `-seed` 参数使用 `DialByFullAddress`
- **稳态连接**: `/connect <NodeID>` 使用 `DialByNodeID`
- **不暴露 Dial Address**: 业务侧只看到 NodeID

**注意**: 本示例仅覆盖 **direct_only** 场景（至少一端可直连）。  
若双方都是 NAT 且无法打洞成功，连接可能失败。

**详细文档**: [chat_public/README.md](chat_public/README.md)

---

### 5️⃣ Relay 示例 - NAT 穿透

**位置**: `examples/relay/`  
**难度**: ⭐⭐⭐⭐ 高级  
**学习时间**: 30 分钟

**你将学到**:
- 什么是 NAT，为什么需要穿透
- Circuit Relay 工作原理
- 如何搭建中继服务器
- 如何通过中继连接 NAT 后的节点

**运行方式**:
```bash
# 第 1 步: 启动 Relay 服务器
go run ./cmd/relay-server -port 4001

# 第 2 步: 启动监听节点
cd examples/relay/
go run main.go -mode listen -relay <relay地址>

# 第 3 步: 启动拨号节点
go run main.go -mode dial -relay <relay地址> -target <目标节点ID>
```

**适合人群**: 需要部署生产环境 P2P 应用的开发者

**详细文档**: [relay/README.md](relay/README.md)

---

---

## 核心概念速查

如果你在示例中遇到不理解的术语，这里有简要解释：

### 节点 (Node/Endpoint)
P2P 网络中的参与者，每个节点有唯一的 ID（类似身份证）。

### 节点 ID (NodeID)
基于公钥派生的唯一标识符，外部表示为 **Base58** 字符串（示例：`5Q2STWvB...`）。

### 连接语义（DialBy 三语义）
示例中所有“连接方式”都映射到三条确定性语义（详见 `docs/04-usage/quickstart.md` 与 `docs/01-design/philosophy/invariants.md`）：

- **DialByNodeID（默认/最纯粹）**：`Connect(ctx, nodeID)`
- **DialByFullAddress（冷启动/分享/Bootstrap）**：`ConnectToAddr(ctx, fullAddr)`
 
> **注意**：`ConnectWithAddrs(ctx, nodeID, dialAddrs)` 属于高级/运维/受控环境用法，不在 `examples/` 中展示。
> 如需该用法，请参考 `docs/04-usage/examples/advanced.md`。

### 监听地址 (Listen Address)
节点在本地绑定的网络地址，格式如 `/ip4/0.0.0.0/udp/4001/quic-v1`。
默认使用 QUIC 传输，提供更好的性能和安全性。

### 通告地址 (Advertised Address)
节点告诉其他节点的可连接地址，可能包含公网 IP。

### 流 (Stream)
节点之间的通信通道，类似 TCP 连接，但基于 QUIC。

### 协议 (Protocol)
定义通信格式的标识符，分为两类：
- **系统协议** (`/dep2p/sys/`): 不需要 Realm 校验的基础设施协议（如 RealmAuth、Identify、Goodbye）
- **应用协议** (`/dep2p/app/`): 需要 Realm 校验的业务协议（如 chat、relay-demo）

注意：并非所有示例都使用应用协议。比如 `echo/` 使用的是系统协议 `SysEcho`，不依赖 JoinRealm。

### Realm（业务隔离租户）
用于实现多租户/多应用隔离的机制：
- 每个节点同一时间只能加入一个 Realm
- 业务 API（Send/Request/Publish/Subscribe）必须先 JoinRealm
- 未 Join 时调用会返回 `endpoint.ErrNotMember`
- 类似于 Kubernetes 的 Namespace 或 云厂商的 VPC

### mDNS
多播 DNS，用于局域网内自动发现其他节点。

### Relay / 中继
帮助 NAT 后的节点建立连接的中间服务器。

### NAT 穿透
让位于路由器后面的节点能够被外部访问的技术。

---

## 常见问题

### Q: 示例运行失败，提示找不到模块

**A**: 确保在项目根目录运行 `go mod download`，或使用 `go run` 时加上 `-mod=mod` 标志。

### Q: 节点无法互相发现（Chat 示例）

**A**: 检查以下几点：
1. 两个节点是否在同一局域网
2. 防火墙是否允许 UDP 5353 端口（mDNS）
3. 网络是否支持多播（某些企业网络会禁用）

### Q: 连接超时或失败

**A**: 可能的原因：
1. 节点 ID 复制错误（ID 很长，建议使用复制粘贴）
2. 地址格式不正确
3. 防火墙阻止了连接
4. 端口被占用

### Q: 示例代码在哪里？

**A**: 
- **基础示例**: `examples/main.go`
- **其他示例**: `examples/<示例名>/main.go`

### Q: 如何在生产环境使用？

**A**: 示例代码用于学习和演示，生产环境需要：
- 完善的错误处理
- 日志记录
- 连接管理
- 安全审计
- 性能优化

建议参考 [docs/](../docs/) 目录的设计文档。

---

## 故障排除

### 编译错误

```bash
# 错误: package xxx is not in GOROOT
# 解决: 下载依赖
go mod download
go mod tidy
```

### 运行时错误

```bash
# 错误: bind: address already in use
# 解决: 端口被占用，使用 -port 0 让系统自动分配端口
go run main.go -port 0
```

### 网络问题

如果在企业网络或特殊环境中遇到问题：

1. **检查防火墙规则**
   ```bash
   # macOS/Linux: 临时关闭防火墙测试
   # 注意：仅用于测试，测试完记得重新开启
   ```

2. **检查网络隔离**
   - Docker 容器默认有网络隔离
   - 虚拟机需要使用桥接模式而非 NAT 模式

3. **VPN 环境**
   - 某些 VPN 会阻止 P2P 流量
   - 参考测试文档中的 VPN 环境说明

### 性能问题

如果遇到性能问题：

1. **延迟高**: 检查是否经过中继（Relay），直连延迟更低
2. **吞吐量低**: 检查网络带宽，QUIC 需要 UDP 支持
3. **CPU 占用高**: 加密操作密集，这是正常现象

---

## 下一步

完成示例学习后，你可以：

1. **阅读设计文档**: [docs/01-design/](../docs/01-design/)
2. **了解实现细节**: [design/implementation/](../design/implementation/)
3. **参与开发**: [CONTRIBUTING.md](../CONTRIBUTING.md)
4. **构建你的应用**: 参考 API 文档

---

## 获取帮助

- **文档**: [docs/](../docs/)
- **测试用例**: [tests/e2e/](../tests/e2e/)
- **API 参考**: [pkg/](../pkg/)

---

## 反馈

如果你发现文档有误或有改进建议，欢迎提 Issue 或 Pull Request！

让我们一起构建更好的 P2P 网络！🚀
