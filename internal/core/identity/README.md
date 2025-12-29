# Identity 身份管理模块

## 概述

**层级**: Tier 1  
**职责**: 管理节点身份，包括密钥对生成、NodeID 派生、签名验证和密钥持久化。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [身份协议规范](../../../docs/01-design/protocols/foundation/01-identity.md) | 核心身份协议 |
| [设备身份协议](../../../docs/01-design/protocols/foundation/03-device-id.md) | 可选扩展 - 多设备支持 |

## 能力清单

### 核心能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| Ed25519 密钥生成 | ✅ 已实现 | 生成 Ed25519 密钥对 |
| NodeID 派生 | ✅ 已实现 | `NodeID = SHA256(PublicKey)` |
| 签名与验证 | ✅ 已实现 | 使用私钥签名，公钥验证 |
| 密钥持久化 (PEM) | ✅ 已实现 | PEM 格式存储私钥 |
| 密钥加载 | ✅ 已实现 | 从文件加载私钥 |
| 自动创建 | ✅ 已实现 | 首次启动自动创建身份 |

### 扩展能力 (可选实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 设备身份 (Device Identity) | ✅ 已实现 | 主从身份模型 |
| 设备证书签发 | ✅ 已实现 | 主身份签发设备证书 |
| 设备证书验证 | ✅ 已实现 | 验证设备证书有效性 |
| 设备撤销检查 | ✅ 已实现 | 检查设备是否被撤销 |
| ECDSA 密钥支持 | ✅ 已实现 | P-256/P-384 曲线 |
| 多密钥类型支持 | ✅ 已实现 | Ed25519, ECDSA |

## 依赖关系

### 接口依赖

```
pkg/types/              → NodeID, KeyType
pkg/interfaces/crypto/  → PublicKey, PrivateKey 接口
pkg/interfaces/identity/ → Identity, IdentityManager 接口
```

### 模块依赖

```
无（Tier 1 基础模块）
```

## 目录结构

```
identity/
├── README.md        # 本文件
├── module.go        # fx 模块定义
├── identity.go      # Identity 实现
├── ed25519.go       # Ed25519 密钥实现
├── ecdsa.go         # ECDSA 密钥实现 (P-256/P-384)
├── device.go        # 设备身份管理
├── nodeid.go        # NodeID 派生
└── storage.go       # 密钥存储
```

## 公共接口

实现 `pkg/interfaces/identity/` 中的接口：

```go
// Identity 节点身份
type Identity interface {
    // ID 返回节点 ID
    ID() types.NodeID
    
    // PublicKey 返回公钥
    PublicKey() crypto.PublicKey
    
    // PrivateKey 返回私钥
    PrivateKey() crypto.PrivateKey
    
    // Sign 签名数据
    Sign(data []byte) ([]byte, error)
    
    // Verify 验证签名
    Verify(data, signature []byte, pubKey crypto.PublicKey) (bool, error)
    
    // KeyType 返回密钥类型
    KeyType() types.KeyType
}

// IdentityManager 身份管理器
type IdentityManager interface {
    // Create 创建新身份
    Create() (Identity, error)
    
    // Load 从文件加载身份
    Load(path string) (Identity, error)
    
    // Save 保存身份到文件
    Save(identity Identity, path string) error
    
    // FromPrivateKey 从私钥创建身份
    FromPrivateKey(key crypto.PrivateKey) (Identity, error)
}
```

## 关键算法

### NodeID 派生 (来自设计文档)

```
NodeID = SHA256(PublicKey)

长度: 32 字节 (256 bits)
编码: Base58
```

```go
func DeriveNodeID(pubKey crypto.PublicKey) types.NodeID {
    hash := sha256.Sum256(pubKey.Bytes())
    var id types.NodeID
    copy(id[:], hash[:])
    return id
}
```

### Ed25519 签名

```go
type ed25519Identity struct {
    privateKey ed25519.PrivateKey  // 64 bytes
    publicKey  ed25519.PublicKey   // 32 bytes
    nodeID     types.NodeID        // 32 bytes
}

func (i *ed25519Identity) Sign(data []byte) ([]byte, error) {
    return ed25519.Sign(i.privateKey, data), nil  // 64 bytes signature
}

func (i *ed25519Identity) Verify(data, sig []byte, pubKey crypto.PublicKey) (bool, error) {
    return ed25519.Verify(pubKey.Bytes(), data, sig), nil
}
```

### 密钥存储格式

```
# PEM 格式存储
-----BEGIN ED25519 PRIVATE KEY-----
...base64 encoded 64 bytes...
-----END ED25519 PRIVATE KEY-----
```

## 设备身份扩展

> 详见: [设备身份协议规范](../../../docs/01-design/protocols/foundation/03-device-id.md)

### 两层身份模型

```
主身份 (Master Identity)
├── 代表用户/组织
├── 可冷存储
└── 用于签发设备证书

设备身份 (Device Identity)
├── 每台设备独立密钥
├── 由主身份签名授权
└── 实际参与网络通信
```

### 设备证书格式

```go
type DeviceCertificate struct {
    Version      uint8           // 证书版本
    MasterID     types.NodeID    // 主身份 NodeID
    DeviceID     types.NodeID    // 设备 NodeID
    DevicePubKey []byte          // 设备公钥 (32 bytes)
    DeviceName   string          // 设备名称
    Capabilities []string        // 设备能力
    ValidFrom    time.Time       // 生效时间
    ValidUntil   time.Time       // 失效时间
    Signature    []byte          // 主身份签名 (64 bytes)
}
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Config *identityif.Config `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    Identity        identityif.Identity        `name:"identity"`
    IdentityManager identityif.IdentityManager `name:"identity_manager"`
}

func Module() fx.Option {
    return fx.Module("identity",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

## 使用示例

```go
// 创建新身份
manager := identity.NewManager(config)
id, err := manager.Create()

// 获取节点 ID
nodeID := id.ID()
fmt.Printf("Node ID: %s\n", nodeID.String())

// 签名数据
sig, err := id.Sign([]byte("hello"))

// 验证签名
valid, err := id.Verify([]byte("hello"), sig, id.PublicKey())

// 保存和加载
manager.Save(id, "/path/to/key.pem")
loaded, err := manager.Load("/path/to/key.pem")
```

## 相关文档

- [身份协议规范](../../../docs/01-design/protocols/foundation/01-identity.md)
- [设备身份协议](../../../docs/01-design/protocols/foundation/03-device-id.md)
- [pkg/interfaces/identity](../../../pkg/interfaces/identity/)
- [pkg/interfaces/crypto](../../../pkg/interfaces/crypto/)
