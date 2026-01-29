# Protobuf 代码生成

## 前置要求

1. 安装 protoc:
   ```bash
   # macOS
   brew install protobuf
   
   # Linux
   apt install protobuf-compiler
   ```

2. 安装 protoc-gen-go:
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   ```

## 使用

```bash
./generate.sh
```

或者使用 go generate:

```bash
cd pkg/lib/proto
go generate ./...
```

## 目录结构

### 主要 Proto 文件（`pkg/lib/proto/`）

所有公共 .proto 文件位于 `pkg/lib/proto/` 目录下，按模块组织：

```
pkg/lib/proto/
├── addressbook/  # 地址簿消息
├── autonat/      # AutoNAT 协议
├── common/       # 公共消息
├── dht/          # DHT 协议
├── gossipsub/    # GossipSub 协议
├── holepunch/    # HolePunch 协议
├── identify/     # Identify 协议
├── key/          # 密钥相关
├── messaging/    # Messaging 协议
├── noise/        # Noise 协议
├── peer/         # 节点相关
├── realm/        # Realm 协议
├── relay/        # Relay 协议
└── rendezvous/   # Rendezvous 协议
```

### Realm 内部 Proto 文件（`internal/realm/*/proto/`）

Realm 模块内部使用的 proto 文件：

```
internal/realm/
├── auth/proto/      # 认证消息
├── gateway/proto/   # 网关消息
├── member/proto/    # 成员消息
└── routing/proto/   # 路由消息
```

> **注意**: 这些内部 proto 文件计划迁移到 `pkg/lib/proto/realm/` 统一管理。
