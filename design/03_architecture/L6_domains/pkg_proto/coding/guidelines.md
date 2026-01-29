# pkg_proto 编码指南

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## Protobuf 编码规范

### 1. 文件组织

**规范**：
- 每个协议一个子目录
- 子目录名使用小写
- proto 文件名与子目录同名

**示例**：
```
pkg/proto/
├── identify/
│   ├── identify.proto      ✅ 正确
│   └── identify.pb.go
├── autonat/
│   ├── autonat.proto       ✅ 正确
│   └── autonat.pb.go
```

---

### 2. Proto 文件头部

**模板**：
```protobuf
// DeP2P <Protocol> Protocol Definitions
// <协议说明>
//
// Protocol: /dep2p/<category>/<protocol>/<version>

syntax = "proto3";

package dep2p.<protocol>;

option go_package = "github.com/dep2p/dep2p/pkg/proto/<protocol>";
```

**示例**：
```protobuf
// DeP2P Identify Protocol Definitions
// 身份识别协议消息定义
//
// Protocol: /dep2p/sys/identify/1.0.0

syntax = "proto3";

package dep2p.identify;

option go_package = "github.com/dep2p/dep2p/pkg/proto/identify";
```

---

### 3. 消息命名

**规范**：
- 使用 PascalCase（大驼峰）
- 消息名应具有描述性
- 避免缩写（除非是通用缩写如 ID, RPC）

**示例**：
```protobuf
// ✅ 好：清晰的命名
message AuthRequest { ... }
message AuthResponse { ... }
message MemberInfo { ... }

// ❌ 坏：模糊的命名
message Req { ... }
message Resp { ... }
message Info { ... }
```

---

### 4. 字段命名

**规范**：
- 使用 snake_case（小写 + 下划线）
- 字段名应具有描述性
- bytes 字段建议加后缀说明含义

**示例**：
```protobuf
// ✅ 好：清晰的字段名
message Message {
  bytes message_id = 1;
  bytes from_peer = 2;
  bytes to_peer = 3;
  uint64 timestamp = 4;
}

// ❌ 坏：模糊的字段名
message Message {
  bytes id = 1;    // 不清楚是什么 ID
  bytes data = 2;  // 不清楚是什么数据
  uint64 t = 3;    // 缩写
}
```

---

### 5. 字段编号分配

**规范**：
```
1-15:     核心字段（1 字节 varint，优先使用）
16-2047:  扩展字段（2 字节 varint）
2048+:    预留（不常用）
```

**示例**：
```protobuf
message Message {
  // 核心字段（1-15）
  bytes id = 1;
  bytes from = 2;
  bytes to = 3;
  
  // 扩展字段（16+）
  uint64 timestamp = 16;
  map<string, bytes> metadata = 17;
}
```

**注意**：
- 编号一旦分配，永不更改
- 删除字段时，保留编号，添加 `reserved`

```protobuf
message Message {
  bytes id = 1;
  reserved 2;  // 旧字段，已删除
  bytes from = 3;
}
```

---

### 6. 枚举定义

**规范**：
- 第一个值必须是 0（proto3 要求）
- 使用大写 + 下划线
- 状态码分组（1xx, 2xx, 3xx）

**示例**：
```protobuf
// ✅ 好：清晰的枚举
enum Status {
  UNUSED = 0;          // proto3 要求
  OK = 100;            // 成功
  E_AUTH_FAILED = 200; // 认证错误
  E_INTERNAL = 300;    // 内部错误
}

// ❌ 坏：没有 0 值
enum Status {
  OK = 1;  // ❌ proto3 需要 0
  ERROR = 2;
}
```

---

### 7. 嵌套消息

**何时使用嵌套**：
- 消息仅在父消息中使用
- 避免顶层命名空间污染

**示例**：
```protobuf
// ✅ 好：相关消息嵌套
message Message {
  enum MessageType {
    REGISTER = 0;
    DISCOVER = 1;
  }
  
  message Register {
    string ns = 1;
  }
  
  MessageType type = 1;
  Register register = 2;
}

// ❌ 坏：所有消息都平铺
enum MessageType { ... }
message MessageRegister { ... }
message MessageDiscover { ... }
message Message { ... }
```

---

### 8. 注释规范

**规范**：
- 每个消息都有注释
- 关键字段有行内注释
- 使用中文注释（DeP2P 内部）

**示例**：
```protobuf
// AuthRequest Realm 认证请求
message AuthRequest {
  // realm_id Realm ID
  bytes realm_id = 1;
  
  // peer_id 请求者节点 ID
  bytes peer_id = 2;
  
  // challenge_response 质询响应（PSK 派生的签名）
  bytes challenge_response = 3;
  
  // timestamp 请求时间戳
  uint64 timestamp = 4;
}
```

---

### 9. 版本兼容性

**规则**：
1. **不删除字段**：使用 `reserved` 标记
2. **不更改字段编号**：编号是协议的一部分
3. **不更改字段类型**：类型变更破坏兼容性
4. **可以添加字段**：新字段使用新编号

**示例**：
```protobuf
// v1.0.0
message Message {
  bytes id = 1;
  bytes from = 2;
}

// v1.1.0 - 添加字段（兼容）
message Message {
  bytes id = 1;
  bytes from = 2;
  uint64 timestamp = 3;  // ✅ 新字段
}

// v2.0.0 - 删除字段（不兼容，需要 reserved）
message Message {
  bytes id = 1;
  reserved 2;  // from 字段已删除
  uint64 timestamp = 3;
}
```

---

### 10. 代码生成

**命令**：
```bash
# 单个文件
protoc --go_out=. --go_opt=paths=source_relative \
  pkg/proto/identify/identify.proto

# 所有文件（通过 go generate）
cd pkg/proto
go generate ./...
```

**generate.go 模板**：
```go
package proto

//go:generate protoc --go_out=. --go_opt=paths=source_relative common/common.proto
//go:generate protoc --go_out=. --go_opt=paths=source_relative peer/peer.proto
// ...
```

---

## 相关文档

- [../design/overview.md](../design/overview.md) - 设计概述
- [../testing/strategy.md](../testing/strategy.md) - 测试策略

---

**最后更新**：2026-01-13
