# Realm Layer (Realm 层)

Realm 层是 DeP2P 的核心创新，提供隔离的 P2P 子网络功能。

## 模块结构

```
realm/
├── doc.go, module.go, realm.go, manager.go, errors.go
├── auth/       # 认证模块
├── member/     # 成员管理
├── routing/    # 域内路由
├── gateway/    # 跨域网关
├── protocol/   # Realm 协议实现
├── services/   # Realm 服务包装
└── interfaces/ # 内部接口
```

## Realm 协议

| 协议 | 说明 |
|------|------|
| `/dep2p/realm/<id>/join/1.0.0` | 加入域请求 |
| `/dep2p/realm/<id>/auth/1.0.0` | 域认证 |
| `/dep2p/realm/<id>/sync/1.0.0` | 成员同步 |

## 认证模式

1. **PSK** - 预共享密钥认证（默认）
2. **Cert** - 证书认证
3. **Custom** - 自定义认证
