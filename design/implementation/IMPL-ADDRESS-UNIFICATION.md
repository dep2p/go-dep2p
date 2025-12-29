# IMPL-ADDRESS-UNIFICATION: 地址类型统一重构

## 1. 问题概述

### 1.1 现状

代码库中存在 **20+ 个分散的 Address 实现**，导致地址处理逻辑混乱：

| 实现类型 | 出现位置 | 数量 |
|---------|---------|------|
| `stringAddress` | dht, discovery, mdns, relay, reachability, endpoint 等 | 10+ |
| `simpleAddress` | endpoint, relay, discovery, tests | 4+ |
| `parsedAddress` | endpoint, address | 2 |
| `mockAddress` | 各测试包 | 5+ |
| 传输相关 | tcpAddress, quicAddress, relayBaseAddress, upnpAddress, natpmpAddress 等 | 6+ |

### 1.2 核心问题

1. **地址格式不统一**：
   - IP:Port 格式：`192.168.1.1:4001`
   - Multiaddr 格式：`/ip4/192.168.1.1/udp/4001/quic-v1`
   - Relay 格式：`/p2p/<relayID>/p2p-circuit/p2p/<destID>`
   - 在边界处需要大量转换代码

2. **规范化入口不唯一**：
   ```
   internal/util/addrutil/     → ToMultiaddr, NormalizeToMultiaddr
   internal/core/address/      → Parser.Parse, parseMultiaddr, parseHostPort
   internal/core/endpoint/     → parseMultiaddr, parseHostPort
   ```
   同一输入在不同地方被规范化成不同输出。

3. **Address 接口过重**：
   - 要求实现 8 个方法：Network, String, Bytes, Equal, IsPublic, IsPrivate, IsLoopback, Multiaddr
   - 很多场景只需要 `String()`，但必须实现全部方法
   - 促使各模块写最小实现（`stringAddress`）

4. **类型安全性不足**：
   ```go
   type AddressInfo struct {
       ID    NodeID
       Addrs []string  // 编译时无法区分 IP:Port 还是 multiaddr
   }
   ```

### 1.3 地址语义分类

| 语义类型 | 用途 | 格式要求 | 示例 |
|---------|------|---------|------|
| **Dial Addr** | 可拨号地址，用于实际连接 | 必须是 canonical multiaddr | `/ip4/1.2.3.4/udp/4001/quic-v1` |
| **Display Addr** | 展示给用户的地址 | IP:Port 或 multiaddr 均可 | `1.2.3.4:4001` |
| **Full Addr** | 身份绑定地址，用于分享/签名 | 带 `/p2p/<nodeID>` 的完整 multiaddr | `/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmNode...` |
| **Relay Addr** | 中继地址 | circuit multiaddr | `/p2p/<relayID>/p2p-circuit/p2p/<destID>` |

---

## 2. 解决方案

### 2.1 设计原则

> 本实施不做向后兼容：一次性切换、删除旧入口、删除旧实现，不保留“暂不改/先兼容”的技术债。

1. **内部唯一表示（Dial Addr）**：所有用于拨号/缓存/签名/地址簿/路由的地址，唯一表示为 **Canonical Multiaddr**（禁止 host:port 进入这些层）
2. **单一解析/规范化入口**：解析与规范化只允许存在于一个包（建议 `internal/core/address`），禁止在 `endpoint`/`transport`/`discovery`/`relay` 各自实现 parse/normalize
3. **强类型穿透**：跨模块传递地址不得使用 `string`；统一使用 `types.Multiaddr`（或 `netaddr.Address` 的实现），在编译期阻断“IP:Port 与 multiaddr 混用”
4. **分层正确**：`pkg/types` 不依赖 `pkg/interfaces/*`。`types.Multiaddr` 只做值对象与解析/构建；`netaddr.Address` 的接口实现放在 `internal/core/address`（或未来的 `pkg/netaddr` 实现包）
5. **彻底删除散落实现**：`stringAddress/simpleAddress/parsedAddress/...` 只允许存在于测试 Mock（且必须在测试目录），生产代码中 0 个

### 2.2 Canonical Multiaddr 规范

```
规范化规则（ParseMultiaddr / CanonicalizeMultiaddr）：

输入                                      → 输出
─────────────────────────────────────────────────────────────────────
1.2.3.4:4001                              → /ip4/1.2.3.4/udp/4001/quic-v1
[::1]:4001                                → /ip6/::1/udp/4001/quic-v1
example.com:4001                          → /dns4/example.com/udp/4001/quic-v1
/ip4/1.2.3.4/udp/4001/quic-v1             → /ip4/1.2.3.4/udp/4001/quic-v1 (保持)
/ip4/1.2.3.4/udp/4001/quic-v1/p2p/Qm...   → /ip4/1.2.3.4/udp/4001/quic-v1/p2p/Qm... (保持)
/p2p/QmRelay/p2p-circuit/p2p/QmDest       → /p2p/QmRelay/p2p-circuit/p2p/QmDest (保持)
/ip4/.../quic-v1/p2p/QmRelay/p2p-circuit  → /ip4/.../quic-v1/p2p/QmRelay/p2p-circuit (保持)

规范化原则（无兼容/无歧义）：
- **拨号层禁止 host:port**：拨号/缓存/地址簿/签名入口必须提供 multiaddr
- **CLI/UI 可接受 host:port**：仅允许在 CLI/UI 边界层接收 host:port，并在边界层根据“节点默认传输配置”明确转换为 multiaddr（该转换不进入 core）
- **Canonical 必须显式包含传输协议**：如 `/udp/<port>/quic-v1` 或 `/tcp/<port>`，禁止“仅 /ip4/x.x.x.x/4001”这类隐式格式
- **Relay 地址必须唯一格式**：统一采用一种 relay multiaddr（见第 4 节），禁止同一语义出现两种等价字符串
- 无效格式直接返回 error（不返回原字符串、不 fallback）
```

### 2.3 统一类型设计

```go
// pkg/types/multiaddr.go

package types

import (
    "net"
    "strings"
)

// Multiaddr 统一地址类型（值对象）
//
// 注意：为保持依赖层级正确，types 包不实现 netaddr.Address 接口；
// netaddr.Address 的实现放在 internal/core/address（或未来 pkg/netaddr 实现包）。
//
// 约束：Multiaddr 的 String() 必须始终返回 canonical multiaddr（以 "/" 开头）。
type Multiaddr string

// ============================================================================
//                              解析/构建
// ============================================================================

// ParseMultiaddr 解析并规范化 multiaddr（仅 multiaddr 输入）。
// 非 multiaddr（如 host:port）应在 CLI/UI 边界层转换后再进入 core。
func ParseMultiaddr(s string) (Multiaddr, error)

// MustParseMultiaddr 解析地址，失败时 panic（仅用于常量初始化）
func MustParseMultiaddr(s string) Multiaddr

// FromHostPort 从 host 和 port 创建 multiaddr（仅供 CLI/UI 边界层使用）
// 需要显式指定传输（避免默认值导致歧义/技术债）
func FromHostPort(host string, port int, transport string) (Multiaddr, error)

// BuildRelayAddr 构建中继地址（canonical relay multiaddr）
func BuildRelayAddr(relayAddr Multiaddr, destID NodeID) (Multiaddr, error)

// ============================================================================
//                              访问方法
// ============================================================================

// String 返回 canonical multiaddr 字符串
func (m Multiaddr) String() string

// HostPort 返回可展示的 host:port（仅用于 UI/日志）
// 对 relay 地址返回空串（强制调用方显式处理，避免误用为拨号输入）
func (m Multiaddr) HostPort() string

// IP 返回 IP 地址（如果可用）
func (m Multiaddr) IP() net.IP

// Port 返回端口号（如果可用）
func (m Multiaddr) Port() int

// PeerID 返回嵌入的 NodeID（如果有 /p2p/<nodeID> 组件）
func (m Multiaddr) PeerID() NodeID

// Transport 返回传输协议
// 返回值: "quic-v1", "tcp", "udp", "p2p-circuit", ""
func (m Multiaddr) Transport() string

// ============================================================================
//                              判断方法
// ============================================================================

// IsRelay 是否是中继地址
func (m Multiaddr) IsRelay() bool

// IsPublic 是否是公网地址
func (m Multiaddr) IsPublic() bool

// IsPrivate 是否是私网地址
func (m Multiaddr) IsPrivate() bool

// IsLoopback 是否是回环地址
func (m Multiaddr) IsLoopback() bool

// IsEmpty 是否为空
func (m Multiaddr) IsEmpty() bool

// ============================================================================
//                              Relay 地址操作
// ============================================================================

// RelayID 返回中继节点 ID（如果是 relay 地址）
func (m Multiaddr) RelayID() NodeID

// DestID 返回目标节点 ID（如果是 relay 地址且包含目标）
func (m Multiaddr) DestID() NodeID

// RelayBaseAddr 返回中继节点的基础地址（去掉 /p2p-circuit 及之后部分）
func (m Multiaddr) RelayBaseAddr() Multiaddr

// WithPeerID 附加 /p2p/<nodeID> 组件
func (m Multiaddr) WithPeerID(nodeID NodeID) Multiaddr

// WithoutPeerID 移除 /p2p/<nodeID> 组件
func (m Multiaddr) WithoutPeerID() Multiaddr
```

### 2.4 统一 netaddr.Address 实现（core 内唯一实现）

```go
// internal/core/address/addr.go

package address

import (
    "github.com/dep2p/go-dep2p/pkg/types"
    "github.com/dep2p/go-dep2p/pkg/interfaces/netaddr"
)

// Addr 通用地址实现（生产代码唯一实现）
//
// - 基于 types.Multiaddr
// - 实现 netaddr.Address
// - 所有生产模块必须复用该实现，禁止自定义 stringAddress/simpleAddress。
type Addr struct {
    ma types.Multiaddr
}

// NewAddr 创建地址
func NewAddr(ma types.Multiaddr) *Addr {
    return &Addr{ma: ma}
}

// Parse 解析 multiaddr 字符串为地址（仅接受 multiaddr）
func Parse(s string) (*Addr, error) {
    ma, err := types.ParseMultiaddr(s)
    if err != nil {
        return nil, err
    }
    return &Addr{ma: ma}, nil
}

// 实现 netaddr.Address 接口
func (a *Addr) Network() string        { return a.ma.Network() }
func (a *Addr) String() string         { return a.ma.String() }
func (a *Addr) Bytes() []byte          { return a.ma.Bytes() }
func (a *Addr) Equal(other netaddr.Address) bool { ... }
func (a *Addr) IsPublic() bool         { return a.ma.IsPublic() }
func (a *Addr) IsPrivate() bool        { return a.ma.IsPrivate() }
func (a *Addr) IsLoopback() bool       { return a.ma.IsLoopback() }
func (a *Addr) Multiaddr() string      { return a.ma.Multiaddr() }

// Multiaddr 返回底层 Multiaddr
func (a *Addr) MA() types.Multiaddr    { return a.ma }
```

---

## 3. 分阶段实施计划

### 单阶段（硬切换）：统一承载类型 + 删除旧入口（不做兼容）

**目标**：一次性消灭散落的 Address 实现/解析入口/字符串传递，统一使用 `types.Multiaddr`（值对象） + `address.Addr`（core 内唯一 `netaddr.Address` 实现）。

**范围**：全仓库（生产代码 + 测试 + 示例 + 文档引用点）

#### 3.1 创建统一类型与唯一解析入口

**文件**：
- `pkg/types/multiaddr.go`：实现 `types.Multiaddr`（只接受 multiaddr 输入、提供 relay 构建/解析、提供展示方法）
- `internal/core/address/addr.go`：实现 `address.Addr`（唯一 `netaddr.Address` 实现）
- `internal/core/address/parse.go`：实现唯一的 `Parse/Normalize`（仅 multiaddr；host:port 必须留在 CLI/UI）

#### 3.2 删除散落实现（必须一次性完成，禁止遗留）

批量删除以下类型：

```
internal/core/discovery/dht/dht_address.go          → stringAddress
internal/core/discovery/service.go                  → simpleAddress
internal/core/discovery/bootstrap/bootstrap.go      → stringAddress
internal/core/discovery/bootstrap/announcer.go      → announcerStringAddress
internal/core/discovery/mdns/mdns.go                → stringAddress
internal/core/discovery/module.go                   → stringAddress
internal/core/endpoint/endpoint_impl.go             → simpleAddress, parsedAddress
internal/core/relay/client.go                       → stringAddress
internal/core/relay/transport.go                    → relayBaseAddress, simpleRelayAddress
internal/core/relay/discovery.go                    → stringAddress
internal/core/relay/client/upgrader.go              → stringAddress
internal/core/reachability/dialback.go              → stringAddress
internal/core/address/addrmgmt/handler.go           → stringAddress
internal/core/nat/holepunch/protocol.go             → StringAddress
internal/core/nat/holepunch/puncher.go              → punchAddress
internal/core/nat/holepunch/tcp_puncher.go          → tcpPunchAddress
...
```

#### 3.3 升级所有数据结构（强制，非可选）

```go
// pkg/types/address.go

// 之前（存在歧义/技术债，必须删除）
type AddressInfo struct {
    ID    NodeID
    Addrs []string
}

// 之后（强制）
type AddressInfo struct {
    ID    NodeID
    Addrs []Multiaddr  // 强类型
}
```

**注意**：这是“彻底重构”的核心之一：不升级数据结构就无法从编译期阻断混用，属于技术债保留，禁止。

#### 3.4 预期效果（验收门槛）

- ✅ 代码库只有 1 个通用 Address 实现
- ✅ 类型安全：编译时保证地址格式正确
- ✅ 维护成本降低：地址逻辑集中在一处
- ✅ 测试覆盖率提升：只需测试一处
- ✅ **仓库验收：生产代码中不得出现 `net.SplitHostPort` / `NormalizeToMultiaddr` / `ToMultiaddr` / 自定义 `stringAddress`**
- ✅ **仓库验收：拨号相关 API 不接受 `string` 地址参数（除 CLI/UI 层）**

---

## 4. Relay 地址统一规范

### 4.1 Relay 地址格式

| 场景 | 格式 | 示例 |
|-----|------|------|
| Relay 自身地址（dialable） | `/<ip|dns>/<...>/<transport>/<port>/<...>/p2p/<relayID>` | `/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay` |
| Relay Circuit 地址（dialable，唯一规范） | `/<relay-addr>/p2p-circuit/p2p/<destID>` | `/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest` |
| Relay Circuit 简写（display-only，禁止用于拨号/缓存） | `/p2p/<relayID>/p2p-circuit/p2p/<destID>` | `/p2p/QmRelay/p2p-circuit/p2p/QmDest` |

**强制约束**：
- 生产代码中，任何用于拨号/缓存/地址簿/签名的 relay 地址 **必须** 为“dialable，唯一规范”的 relay circuit multiaddr（带中继的底层可达地址）。
- “简写（display-only）”仅可用于日志输出或用户界面展示，任何进入 core 的路径必须先解析为 dialable 形式，否则直接报错。

### 4.2 Relay 地址处理集中化

**当前问题**：relay 地址构建/解析分散在多处

```
pkg/types/address.go           → BuildRelayAddr, BuildRelayAddrWithIDs, ParseRelayAddr
internal/core/relay/client.go  → relayAddr.String()
internal/core/address/parser.go → parseMultiaddr (处理 relay)
internal/util/addrutil/         → IsRelayCircuitAddr
```

**目标**：集中到 `pkg/types/multiaddr.go`

```go
// pkg/types/multiaddr.go

// BuildRelayAddr 构建中继地址（dialable，唯一规范）
//
// 参数：
//   - relayAddr: 中继节点地址（必须是 Multiaddr 且包含 /p2p/<relayID>）
//   - destID: 目标节点 ID（可为空）
//
// 返回格式（强制）：
//   - <relayAddr>/p2p-circuit/p2p/<destID>
func BuildRelayAddr(relayAddr Multiaddr, destID NodeID) (Multiaddr, error)

// ParseRelayAddr 解析中继地址
//
// 返回：
//   - relayBaseAddr: 中继节点的基础地址
//   - relayID: 中继节点 ID
//   - destID: 目标节点 ID（可能为空）
func ParseRelayAddr(addr Multiaddr) (relayBaseAddr Multiaddr, relayID NodeID, destID NodeID, err error)

// IsRelayAddr 判断是否是中继地址
func IsRelayAddr(addr Multiaddr) bool
```

---

## 5. 测试策略

### 5.1 规范化与类型安全测试（强制）

```go
// internal/core/address/parse_test.go

// 测试规范化一致性
func TestNormalizeConsistency(t *testing.T) {
    // 同一 multiaddr 输入应在不同路径保持幂等（Parse/Normalize 结果相同）
    inputs := []string{
        "/ip4/1.2.3.4/udp/4001/quic-v1",
        "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmNode",
        "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest",
    }
    
    for _, input := range inputs {
        ma1, err := types.ParseMultiaddr(input)
        require.NoError(t, err)
        ma2, err := types.ParseMultiaddr(ma1.String())
        require.NoError(t, err)
        require.Equal(t, ma1, ma2)
    }
}

// 测试幂等性（multiaddr 输入幂等）
func TestNormalizeIdempotent(t *testing.T) {
    input := "/ip4/1.2.3.4/udp/4001/quic-v1"
    ma1, _ := types.ParseMultiaddr(input)
    ma2, _ := types.ParseMultiaddr(ma1.String())
    require.Equal(t, ma1, ma2)
}
```

### 5.2 核心类型测试（强制）

```go
// pkg/types/multiaddr_test.go

func TestMultiaddrParse(t *testing.T) { ... }
func TestMultiaddrHostPort(t *testing.T) { ... }
func TestMultiaddrIsRelay(t *testing.T) { ... }
func TestMultiaddrBuildRelay(t *testing.T) { ... }

// internal/core/address/addr_test.go
func TestAddrImplementsNetaddrAddress(t *testing.T) { ... }
```

### 5.3 Lint 测试

```go
// tests/invariants/address_lint_test.go

// 检查是否还有散落的 Address 实现
func TestNoScatteredAddressImplementations(t *testing.T) {
    // 扫描代码库，检查 stringAddress, simpleAddress 等是否已删除
    forbidden := []string{
        "type stringAddress struct",
        "type simpleAddress struct",
        "type mockAddress struct",
    }
    // ...
}
```

---

## 6. 迁移指南

### 6.1 从 stringAddress 迁移

```go
// 之前
type stringAddress struct{ s string }
func (a *stringAddress) String() string { return a.s }
// ... 其他方法

addr := &stringAddress{s: "/ip4/1.2.3.4/udp/4001/quic-v1"}

// 之后
import "github.com/dep2p/go-dep2p/internal/core/address"

addr, _ := address.Parse("/ip4/1.2.3.4/udp/4001/quic-v1")
```

### 6.2 从 []string 迁移

```go
// 之前
addrs := []string{"/ip4/1.2.3.4/udp/4001/quic-v1"}
for _, s := range addrs {
    addr := &stringAddress{s: s}
    // ...
}

// 之后（强制：改类型）
addrs := []types.Multiaddr{
    types.MustParseMultiaddr("/ip4/1.2.3.4/udp/4001/quic-v1"),
}
for _, ma := range addrs {
    addr := address.NewAddr(ma)
    // ...
}
```

### 6.3 IP:Port 输入处理

```go
// 之后（彻底重构的口径）
// - core/transport/discovery/relay/endpoint 等所有生产模块：不再接受 host:port
// - host:port 仅允许出现在 CLI/UI，且必须显式指定 transport 后转换为 multiaddr
ma, err := types.FromHostPort(host, port, "udp/quic-v1")
if err != nil {
    return fmt.Errorf("invalid host:port: %w", err)
}
// ma.String() 作为唯一拨号输入进入 core
```

---

## 7. 时间线与里程碑

### 单阶段硬切换（建议 2-3 周）

| 任务 | 预估工时 | 优先级 |
|-----|---------|-------|
| 实现 `types.Multiaddr`（仅 multiaddr 输入） | 4h | P0 |
| 实现 `address.Addr`（唯一 netaddr.Address 实现） | 2h | P0 |
| 删除所有散落 Address 实现 + 删除散落 parse/normalize | 12h | P0 |
| 全量升级数据结构（`[]string` → `[]types.Multiaddr`） | 6h | P0 |
| 统一 Relay dialable 格式（只保留一种） | 4h | P0 |
| 删除 `internal/util/addrutil` 中与地址规范化重复/冲突的函数与调用点 | 6h | P0 |
| 全量测试 + 修复（含 e2e） | 12h | P0 |

---

## 8. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|-----|------|---------|
| 大规模重构引入回归 | 高 | 以“编译期强约束 + Lint + 全量测试”作为门禁，禁止保留旧入口 |
| 现有测试/示例依赖旧实现 | 中 | 同步升级测试/示例；允许一次性破坏但必须在同一改动中修复 |
| Relay 地址格式不兼容 | 中 | 明确“唯一 dialable 格式”；任何简写只用于 display，进入 core 必须报错 |
| 性能影响（解析开销） | 低 | `types.Multiaddr` 做最小解析 + 可选缓存；用基准测试验证并优化 |

---

## 9. 相关文档

- [IMPL-1227-api-layer-refactor.md](./IMPL-1227-api-layer-refactor.md) - API 层重构
- 地址管理设计文档：以 `docs/` 目录为准（`_docs/` 为弃用目录，不应作为来源引用）
- [Relay 协议设计](../protocols/transport/relay.md)

---

## 附录：散落 Address 实现清单

```bash
# 执行以下命令查找所有 Address 实现
grep -rn "type.*Address struct" --include="*.go" | grep -v "_test.go" | grep -v "vendor"
```

**当前发现（按包分类）**：

```
internal/core/endpoint/
  - simpleAddress
  - parsedAddress

internal/core/relay/
  - relayBaseAddress
  - simpleRelayAddress
  - stringAddress (discovery.go)

internal/core/discovery/
  - stringAddress (dht/, bootstrap/, mdns/, service.go, module.go)
  - simpleAddress (service.go)

internal/core/address/
  - ParsedAddress (parser.go)
  - stringAddress (addrmgmt/)

internal/core/reachability/
  - stringAddress (dialback.go)

internal/core/nat/
  - punchAddress (holepunch/)
  - tcpPunchAddress (holepunch/)
  - StringAddress (holepunch/protocol.go)
  - natpmpAddress (natpmp/)
  - upnpAddress (upnp/)
  - httpAddress (http/)

internal/core/transport/
  - Address (tcp/, quic/)

tests/
  - mockAddress (多处)
  - simpleAddress (e2e/)
```

