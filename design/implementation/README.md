# 实现映射

本目录包含 DeP2P 设计到代码的映射文档，确保设计与实现的可追踪性。

---

## 概述

### 实现映射的目的

实现映射文档建立了**设计文档**与**代码实现**之间的追踪关系：

```mermaid
flowchart LR
    subgraph Design [设计文档]
        REQ["需求规范<br/>requirements/"]
        ARCH["架构设计<br/>architecture/"]
        PROTO["协议规范<br/>protocols/"]
        ADR["架构决策<br/>adr/"]
    end
    
    subgraph Impl [实现映射]
        MAP["模块映射<br/>module-map.md"]
        STATUS["实现状态<br/>status.md"]
        FXLC["fx 生命周期<br/>fx-lifecycle.md"]
    end
    
    subgraph Code [代码实现]
        PKG["pkg/interfaces/"]
        INTERNAL["internal/core/"]
        APP["internal/app/"]
    end
    
    Design --> Impl
    Impl --> Code
```

### 映射的价值

| 价值 | 描述 |
|------|------|
| 可追踪性 | 从需求到代码的完整追踪链 |
| 一致性检查 | 验证实现是否符合设计 |
| 变更影响 | 评估设计变更对代码的影响 |
| 新人入门 | 快速了解代码组织结构 |
| 代码审查 | 提供审查的设计依据 |

---

## 文档导航

| 文档 | 描述 |
|------|------|
| [module-map.md](module-map.md) | 设计组件 → Go 包/目录映射 |
| [status.md](status.md) | 实现状态（链接 PR/Issue） |
| [fx-lifecycle.md](fx-lifecycle.md) | fx 模块生命周期 |

---

## 映射关系

### 设计到代码的映射方式

```mermaid
flowchart TB
    subgraph DesignLayer [设计层]
        Component["设计组件<br/>(architecture/components.md)"]
        Protocol["协议规范<br/>(protocols/*.md)"]
        Requirement["需求规范<br/>(requirements/*.md)"]
    end
    
    subgraph InterfaceLayer [接口层]
        Interface["Go 接口<br/>(pkg/interfaces/)"]
    end
    
    subgraph ImplLayer [实现层]
        CoreImpl["核心实现<br/>(internal/core/)"]
        AppImpl["应用编排<br/>(internal/app/)"]
    end
    
    Component --> |"定义接口"| Interface
    Protocol --> |"实现协议"| CoreImpl
    Requirement --> |"满足需求"| CoreImpl
    
    Interface --> |"实现接口"| CoreImpl
    CoreImpl --> |"组装"| AppImpl
```

### 映射层次

| 设计层次 | 代码位置 | 说明 |
|---------|---------|------|
| 设计组件 | `pkg/interfaces/` | 接口定义 |
| 协议规范 | `internal/core/` | 协议实现 |
| 架构层次 | `internal/app/` | 模块组装 |
| 用户 API | 根目录 | Facade API |

---

## 代码结构概览

### 目录结构

```
dep2p/
├── pkg/                          # 公共包（对外暴露）
│   ├── interfaces/               # 接口定义（17+ 模块）
│   │   ├── address/              # 地址管理接口
│   │   ├── bandwidth/            # 带宽统计接口
│   │   ├── connmgr/              # 连接管理接口
│   │   ├── discovery/            # 发现服务接口
│   │   ├── endpoint/             # 端点接口
│   │   ├── identity/             # 身份管理接口
│   │   ├── liveness/             # 存活检测接口
│   │   ├── messaging/            # 消息服务接口
│   │   ├── muxer/                # 多路复用接口
│   │   ├── nat/                  # NAT 穿透接口
│   │   ├── netreport/            # 网络诊断接口
│   │   ├── protocol/             # 协议管理接口
│   │   ├── reachability/         # 可达性接口
│   │   ├── realm/                # 领域管理接口
│   │   ├── relay/                # 中继服务接口
│   │   ├── security/             # 安全层接口
│   │   └── transport/            # 传输层接口
│   ├── types/                    # 纯类型定义
│   └── proto/                    # Protobuf 定义
│
├── internal/                     # 内部实现
│   ├── app/                      # 应用编排层
│   │   ├── bootstrap.go          # 模块组装
│   │   ├── lifecycle.go          # 生命周期管理
│   │   └── modulesets.go         # 模块集合
│   ├── config/                   # 配置管理
│   └── core/                     # 核心实现（17+ 模块）
│       ├── address/              # 地址管理实现
│       ├── bandwidth/            # 带宽统计实现
│       ├── connmgr/              # 连接管理实现
│       ├── discovery/            # 发现服务实现
│       ├── endpoint/             # 端点实现
│       ├── identity/             # 身份管理实现
│       ├── liveness/             # 存活检测实现
│       ├── messaging/            # 消息服务实现
│       ├── muxer/                # 多路复用实现
│       ├── nat/                  # NAT 穿透实现
│       ├── netreport/            # 网络诊断实现
│       ├── protocol/             # 协议管理实现
│       ├── reachability/         # 可达性实现
│       ├── realm/                # 领域管理实现
│       ├── relay/                # 中继服务实现
│       ├── security/             # 安全层实现
│       └── transport/            # 传输层实现
│
├── dep2p.go                      # 用户 API 入口
├── node.go                       # Node Facade
├── options.go                    # 配置选项
└── presets.go                    # 预设配置
```

### 分层架构

```mermaid
graph TB
    subgraph UserCode [用户代码]
        User[用户应用]
    end
    
    subgraph UserAPI [用户 API]
        DepAPI["dep2p.go<br/>node.go<br/>options.go<br/>presets.go"]
    end
    
    subgraph AppLayer [应用编排层]
        Bootstrap["internal/app/<br/>bootstrap.go"]
    end
    
    subgraph InterfaceLayer [接口契约层]
        Interfaces["pkg/interfaces/<br/>17+ 模块接口"]
    end
    
    subgraph CoreLayer [组件实现层]
        Core["internal/core/<br/>17+ 模块实现"]
    end
    
    User -->|调用| DepAPI
    DepAPI -->|创建| Bootstrap
    Bootstrap -->|组装| Core
    Core -->|实现| Interfaces
```

---

## 追踪方法

### 如何追踪实现状态

1. **需求追踪**: 查看 `status.md` 中的需求实现状态
2. **组件追踪**: 查看 `module-map.md` 中的组件到代码映射
3. **测试追踪**: 查看 `../testing/` 中的测试覆盖情况

### 追踪链路

```mermaid
flowchart LR
    REQ["REQ-XXX-XXX<br/>需求规范"] --> |"实现"| IMPL["internal/core/xxx/<br/>实现代码"]
    IMPL --> |"测试"| TEST["tests/requirements/<br/>测试代码"]
    TEST --> |"验证"| RESULT["测试结果"]
    
    REQ --> |"关联"| INV["INV-XXX<br/>不变量"]
    INV --> |"验证"| INVTEST["tests/invariants/<br/>不变量测试"]
```

---

## 与其他文档的关系

```mermaid
flowchart TB
    subgraph DesignDocs [设计文档]
        README["design/README.md"]
        ARCH["architecture/"]
        REQ["requirements/"]
        PROTO["protocols/"]
        ADR["adr/"]
        INV["invariants/"]
    end
    
    subgraph ImplDocs [实现映射]
        IMPL["implementation/"]
    end
    
    subgraph TestDocs [测试追踪]
        TEST["testing/"]
    end
    
    README --> ARCH
    README --> REQ
    README --> PROTO
    README --> ADR
    README --> INV
    README --> IMPL
    README --> TEST
    
    ARCH --> |"映射"| IMPL
    REQ --> |"追踪"| IMPL
    IMPL --> |"验证"| TEST
```

| 关系 | 说明 |
|------|------|
| 架构设计 → 实现映射 | 设计组件映射到代码 |
| 需求规范 → 实现映射 | 需求实现状态追踪 |
| 实现映射 → 测试追踪 | 实现代码的测试覆盖 |

---

## 相关文档

- [设计文档导航](../README.md)
- [架构设计](../architecture/README.md)
- [需求规范](../requirements/README.md)
- [测试追踪](../testing/README.md)
