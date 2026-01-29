# Core Identity 模块

> **版本**: v1.2.0  
> **更新日期**: 2026-01-19  
> **定位**: 身份管理（Core Layer）

---

## 模块概述

core_identity 负责 DeP2P 的密码学身份管理，包括密钥对生成、PeerID 派生和签名验证。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/identity/` |
| **Fx 模块** | `fx.Module("identity")` |
| **状态** | ✅ 已实现 |
| **依赖** | 无（最底层模块） |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        core_identity 职责                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 密钥对管理                                                               │
│     • Ed25519 密钥对生成（默认）                                            │
│     • Secp256k1 支持                                                        │
│     • RSA 支持                                                              │
│     • 私钥安全存储                                                          │
│     • 公钥导出                                                              │
│                                                                             │
│  2. PeerID 派生                                                             │
│     • 公钥 → Multihash                                                      │
│     • Base58 编码                                                           │
│     • PeerID 验证                                                           │
│                                                                             │
│  3. 签名与验证                                                              │
│     • 数据签名                                                              │
│     • 签名验证                                                              │
│     • 消息认证                                                              │
│                                                                             │
│  4. ★ 设备身份管理（v1.2.0 新增）                                           │
│     • 设备证书创建与验证                                                     │
│     • 设备与节点身份绑定                                                     │
│     • 证书续期与过期管理                                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 支持的密钥类型

| 密钥类型 | 默认 | 说明 |
|----------|------|------|
| Ed25519 | ✅ | 推荐，快速且安全 |
| Secp256k1 | | 区块链兼容 |
| RSA | | 传统兼容（2048/4096 bit） |

---

## 公共接口

```go
// pkg/interfaces/identity.go

// Identity 身份接口
type Identity interface {
    // PeerID 返回节点 ID（= NodeID，Base58 编码字符串）
    PeerID() string
    
    // PublicKey 返回公钥
    PublicKey() PublicKey
    
    // PrivateKey 返回私钥
    PrivateKey() PrivateKey
    
    // Sign 使用私钥签名数据
    Sign(data []byte) ([]byte, error)
    
    // Verify 验证签名
    Verify(data, sig []byte) (bool, error)
}
```

> **NodeID = PeerID**：在 DeP2P 中，NodeID 是 PeerID 的类型别名。
> PeerID 采用 Base58 编码的 SHA256 公钥哈希，用于路由和身份统一表示。

---

## PeerID 格式

```
PeerID 派生流程（DeP2P 原生格式）：

  PublicKey → SHA256 → Base58

  PeerID = Base58(SHA256(pubKey))

示例:
  5dsgvJGnvAfiR3K6HCBc4hcokSfmjj

格式说明:
  • DeP2P 原生格式: Base58(SHA256(pubKey)) - 32 字节哈希
  • 也支持 Multihash 格式（用于 libp2p 兼容）
```

---

## 目录结构

```
internal/core/identity/
├── doc.go              # 包文档
├── module.go           # Fx 模块定义
├── identity.go         # Identity 实现（实现 pkgif.Identity）
├── key.go              # 密钥生成与适配器
├── peerid.go           # PeerID 派生（包装 pkg/lib/crypto）
├── signing.go          # 签名与验证
├── errors.go           # 错误定义
├── device.go           # 设备身份与证书管理
└── *_test.go           # 单元测试
```

---

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `KeyType` | Ed25519 | 密钥类型 |
| `KeyBits` | 2048 | RSA 密钥位数 |
| `PrivKeyPath` | 无 | 私钥文件路径（可选） |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_security](../core_security/) | 安全层（使用 Identity） |
| [core_peerstore](../core_peerstore/) | 节点存储（存储公钥） |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-25
