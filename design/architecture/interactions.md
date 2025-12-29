# 组件交互

本文档描述 DeP2P 核心组件间的交互流程和时序。

---

## 交互概览

```mermaid
flowchart LR
    subgraph User [用户操作]
        U1["启动节点"]
        U2["加入 Realm"]
        U3["发送消息"]
    end
    
    subgraph System [系统响应]
        S1["初始化组件"]
        S2["验证/注册"]
        S3["路由/传输"]
    end
    
    U1 --> S1
    U2 --> S2
    U3 --> S3
```

---

## 节点启动流程

### 时序图

```mermaid
sequenceDiagram
    participant User as 用户
    participant Node as Node
    participant Endpoint as Endpoint
    participant Transport as Transport
    participant DHT as DHT
    participant NAT as NAT
    participant Bootstrap as Bootstrap
    
    User->>Node: NewNode(opts)
    Node->>Endpoint: New()
    
    Endpoint->>Transport: Listen()
    Transport-->>Endpoint: Listener
    
    Endpoint->>NAT: Start()
    NAT->>NAT: 检测 NAT 类型
    NAT-->>Endpoint: NAT Type
    
    Endpoint->>Bootstrap: Start()
    Bootstrap->>DHT: Connect to seeds
    DHT-->>Bootstrap: Connected
    Bootstrap-->>Endpoint: Ready
    
    Endpoint-->>Node: Ready
    Node-->>User: node
```

### 流程说明

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           节点启动流程                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. 用户调用 NewNode(opts)                                                   │
│     └─ 解析配置选项                                                          │
│                                                                              │
│  2. 创建 Endpoint                                                            │
│     ├─ 生成或加载身份密钥                                                    │
│     └─ 初始化内部组件                                                        │
│                                                                              │
│  3. 启动 Transport                                                           │
│     ├─ 绑定监听地址                                                          │
│     └─ 准备接受连接                                                          │
│                                                                              │
│  4. 启动 NAT                                                                 │
│     ├─ 检测 NAT 类型                                                         │
│     ├─ 尝试 UPnP/NAT-PMP 端口映射                                            │
│     └─ 发现外部地址                                                          │
│                                                                              │
│  5. 启动 Bootstrap                                                           │
│     ├─ 连接种子节点                                                          │
│     └─ 加入 DHT 网络                                                         │
│                                                                              │
│  6. 返回就绪的 Node                                                          │
│     └─ Layer 1 自动就绪                                                      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 连接建立流程

### DialByNodeID 时序图

```mermaid
sequenceDiagram
    participant User as 用户
    participant Node as Node
    participant ConnMgr as ConnManager
    participant AddrBook as AddressBook
    participant DHT as DHT
    participant Transport as Transport
    
    User->>Node: Connect(nodeID)
    Node->>ConnMgr: GetConnection(nodeID)
    
    alt 已有连接
        ConnMgr-->>Node: connection
        Node-->>User: connection
    else 无连接
        ConnMgr-->>Node: nil
        Node->>AddrBook: Addrs(nodeID)
        
        alt 有地址
            AddrBook-->>Node: addrs
        else 无地址
            Node->>DHT: FindPeer(nodeID)
            DHT-->>Node: addrs
        end
        
        Node->>Transport: Dial(addr, nodeID)
        Transport->>Transport: TLS 握手
        Transport->>Transport: 验证身份
        Transport-->>Node: connection
        Node->>ConnMgr: AddConnection(conn)
        Node-->>User: connection
    end
```

### DialByFullAddress 时序图

```mermaid
sequenceDiagram
    participant User as 用户
    participant Node as Node
    participant Transport as Transport
    
    User->>Node: ConnectToAddr(fullAddr)
    Node->>Node: 解析 Full Address
    Node->>Node: 提取 NodeID 和 Dial Addrs
    Node->>Transport: Dial(addr, nodeID)
    Transport->>Transport: TLS 握手
    Transport->>Transport: 验证身份
    Transport-->>Node: connection
    Node-->>User: connection
```

### 连接失败处理

```mermaid
flowchart TD
    Start["连接请求"] --> Direct{"直连可达?"}
    Direct -->|成功| Success["连接成功"]
    Direct -->|失败| Punch{"打洞成功?"}
    Punch -->|成功| Success
    Punch -->|失败| RelayCheck{"有 Relay?"}
    RelayCheck -->|是| RelayConn["Relay 连接"]
    RelayCheck -->|否| Fail["连接失败"]
    RelayConn -->|成功| Success
    RelayConn -->|失败| Fail
```

---

## Realm 加入流程

### 时序图

```mermaid
sequenceDiagram
    participant User as 用户
    participant Node as Node
    participant RealmMgr as RealmManager
    participant RealmAuth as RealmAuth
    participant DHT as DHT
    
    User->>Node: JoinRealm(realmID, opts)
    Node->>RealmMgr: JoinRealm(realmID, opts)
    
    RealmMgr->>RealmMgr: 检查当前状态
    
    alt 已在其他 Realm
        RealmMgr-->>Node: ErrAlreadyJoined
        Node-->>User: error
    else 未加入 Realm
        RealmMgr->>RealmAuth: Authenticate(realmID, credential)
        
        alt Public Realm
            RealmAuth-->>RealmMgr: ok
        else Protected Realm
            RealmAuth->>RealmAuth: 验证邀请码
            RealmAuth-->>RealmMgr: ok/error
        else Private Realm
            RealmAuth->>RealmAuth: 验证签名
            RealmAuth-->>RealmMgr: ok/error
        end
        
        RealmMgr->>DHT: Announce(realmID)
        DHT-->>RealmMgr: ok
        
        RealmMgr-->>Node: ok
        Node-->>User: ok
    end
```

### 状态转换

```mermaid
stateDiagram-v2
    [*] --> NotMember: 节点启动
    
    NotMember --> Joining: JoinRealm()
    Joining --> Member: 验证通过
    Joining --> NotMember: 验证失败
    
    Member --> Leaving: LeaveRealm()
    Leaving --> NotMember: 完成
    
    Member --> Switching: SwitchRealm()
    Switching --> Member: 成功
    Switching --> Member: 失败（保持原 Realm）
```

---

## 消息发送流程

### Send 时序图

```mermaid
sequenceDiagram
    participant User as 用户
    participant Node as Node
    participant RealmMgr as RealmManager
    participant Messaging as Messaging
    participant ConnMgr as ConnManager
    participant Transport as Transport
    
    User->>Node: Send(peerID, proto, data)
    Node->>RealmMgr: IsMember()
    
    alt 未加入 Realm
        RealmMgr-->>Node: false
        Node-->>User: ErrNotMember
    else 已加入 Realm
        RealmMgr-->>Node: true
        Node->>Messaging: Send(peerID, proto, data)
        Messaging->>ConnMgr: GetConnection(peerID)
        
        alt 无连接
            ConnMgr-->>Messaging: nil
            Messaging->>Node: Connect(peerID)
            Node-->>Messaging: connection
        else 有连接
            ConnMgr-->>Messaging: connection
        end
        
        Messaging->>Transport: OpenStream(proto)
        Transport-->>Messaging: stream
        Messaging->>Transport: Write(data)
        Transport-->>Messaging: ok
        Messaging->>Transport: Close()
        Messaging-->>Node: ok
        Node-->>User: ok
    end
```

### Request 时序图

```mermaid
sequenceDiagram
    participant User as 用户
    participant Node as Node
    participant RealmMgr as RealmManager
    participant Messaging as Messaging
    participant Transport as Transport
    participant Remote as 远程节点
    
    User->>Node: Request(peerID, proto, req)
    Node->>RealmMgr: IsMember()
    
    alt 未加入 Realm
        RealmMgr-->>Node: false
        Node-->>User: ErrNotMember
    else 已加入 Realm
        RealmMgr-->>Node: true
        Node->>Messaging: Request(peerID, proto, req)
        Messaging->>Transport: OpenStream(proto)
        Transport-->>Messaging: stream
        
        Messaging->>Remote: Write(req)
        Remote-->>Messaging: Read(resp)
        
        Messaging->>Transport: Close()
        Messaging-->>Node: resp
        Node-->>User: resp
    end
```

---

## 发现流程

### DHT 发现时序图

```mermaid
sequenceDiagram
    participant Node as 本地节点
    participant DHT as DHT
    participant Peer1 as 节点 1
    participant Peer2 as 节点 2
    participant Target as 目标节点
    
    Node->>DHT: FindPeer(targetID)
    DHT->>Peer1: FIND_NODE(targetID)
    Peer1-->>DHT: closer peers
    
    DHT->>Peer2: FIND_NODE(targetID)
    Peer2-->>DHT: target addrs
    
    DHT-->>Node: AddrInfo(targetID, addrs)
```

### mDNS 发现时序图

```mermaid
sequenceDiagram
    participant Node as 本地节点
    participant mDNS as mDNS
    participant LAN as 局域网
    participant Peer as 局域网节点
    
    Node->>mDNS: Start()
    mDNS->>LAN: 广播查询
    Peer-->>mDNS: 响应
    mDNS-->>Node: PeerFound(peerID, addrs)
```

---

## 错误处理流程

### 错误分类

```mermaid
flowchart TD
    Error["错误发生"] --> Type{"错误类型?"}
    
    Type -->|连接错误| ConnErr["连接层错误"]
    Type -->|Realm 错误| RealmErr["Realm 层错误"]
    Type -->|协议错误| ProtoErr["协议层错误"]
    
    ConnErr --> CE1["ErrNotConnected"]
    ConnErr --> CE2["ErrConnectionRefused"]
    ConnErr --> CE3["ErrTimeout"]
    
    RealmErr --> RE1["ErrNotMember"]
    RealmErr --> RE2["ErrAlreadyJoined"]
    RealmErr --> RE3["ErrInvalidCredential"]
    
    ProtoErr --> PE1["ErrUnsupportedProtocol"]
    ProtoErr --> PE2["ErrStreamReset"]
```

### 重试策略

```mermaid
flowchart TD
    Start["操作失败"] --> Retry{"可重试?"}
    Retry -->|是| Count{"重试次数 < 最大?"}
    Retry -->|否| Fail["返回错误"]
    Count -->|是| Backoff["等待退避时间"]
    Count -->|否| Fail
    Backoff --> Attempt["重试"]
    Attempt --> Success{"成功?"}
    Success -->|是| Done["完成"]
    Success -->|否| Retry
```

---

## 关键交互总结

| 流程 | 涉及层 | 关键组件 | 前置条件 |
|------|--------|----------|----------|
| 节点启动 | L1 | Transport, DHT, NAT | 无 |
| 连接建立 | L1 | ConnMgr, AddrBook, DHT | 节点就绪 |
| Realm 加入 | L2 | RealmMgr, RealmAuth | 节点就绪 |
| 消息发送 | L3 | Messaging, ConnMgr | 已加入 Realm |
| 发布订阅 | L3 | PubSub, DHT | 已加入 Realm |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [架构总览](overview.md) | 高层视图和设计理念 |
| [三层架构详解](layers.md) | 各层职责和边界 |
| [核心组件](components.md) | 各组件的职责和接口 |
