# Realm Auth - 认证机制

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **架构层**: Realm Layer

---

## 概述

`auth` 提供 Realm 层的成员认证功能，支持多种认证模式。

**核心功能**:
- 🔑 PSK 认证 - 预共享密钥认证（推荐）
- 📜 证书认证 - X.509 证书认证
- 🎯 自定义认证 - 可扩展认证逻辑
- 🛡️ 防重放攻击 - Nonce + 时间戳验证

---

## 快速开始

### PSK 认证（推荐）

```go
import "github.com/dep2p/go-dep2p/internal/realm/auth"

// 派生 RealmID
psk := []byte("my-secret-key")
realmID := auth.DeriveRealmID(psk)

// 创建认证器
authenticator, err := auth.NewPSKAuthenticator(psk, "peer123")
if err != nil {
    log.Fatal(err)
}
defer authenticator.Close()

// 生成证明
proof, err := authenticator.GenerateProof(ctx)

// 验证证明
valid, err := authenticator.Authenticate(ctx, "peer456", proof)
if !valid {
    log.Println("认证失败")
}
```

### 证书认证

```go
authenticator, err := auth.NewCertAuthenticator(
    "/path/to/cert.pem",
    "/path/to/key.pem",
    "peer123",
)
```

### 自定义认证

```go
validator := func(ctx context.Context, peerID string, proof []byte) (bool, error) {
    return string(proof) == "secret-token", nil
}
authenticator := auth.NewCustomAuthenticator("realm123", "peer123", validator)
```

---

## 认证模式

### PSK 模式

**密钥派生**：
- `RealmID = HKDF(PSK, salt="dep2p-realm-id-v1", info=SHA256(PSK))`
- `AuthKey = HKDF(PSK, salt="dep2p-auth-key-v1", info=RealmID)`

**认证流程**：
1. 客户端发送 `AuthRequest`
2. 服务端返回 `AuthChallenge`（nonce）
3. 客户端计算 `proof = HMAC-SHA256(AuthKey, nonce||peerID||timestamp)`
4. 服务端验证 proof

### 证书模式

1. 客户端发送证书
2. 服务端验证证书链
3. 检查有效期和吊销状态
4. 返回认证结果

---

## 安全特性

| 特性 | 说明 |
|------|------|
| HKDF 密钥派生 | RFC 5869 |
| HMAC-SHA256 | 消息认证 |
| crypto/rand | 随机 nonce |
| 时间戳验证 | 防重放攻击 |
| 证书链验证 | 完整性校验 |

---

## 配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `PSK` | - | 预共享密钥（16-64 字节） |
| `Timeout` | `30s` | 认证超时 |
| `ReplayWindow` | `5min` | 重放窗口 |
| `MaxRetries` | `3` | 最大重试次数 |

---

## 测试

```bash
go test -v ./internal/realm/auth/...
go test -cover ./internal/realm/auth/...
```

---

## 相关文档

- [doc.go](doc.go) - 包文档
- [QUALITY_CHECK.md](QUALITY_CHECK.md) - 质量检查

---

**最后更新**: 2026-01-20
