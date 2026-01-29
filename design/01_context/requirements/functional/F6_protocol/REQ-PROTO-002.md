# REQ-PROTO-002: 消息格式

## 1. 元数据

| 属性 | 值 |
|------|---|
| **ID** | REQ-PROTO-002 |
| **标题** | 消息格式 |
| **类型** | generic |
| **层级** | F6: 协议层 |
| **优先级** | P2 |
| **状态** | draft |
| **创建日期** | 2026-01-11 |
| **更新日期** | 2026-01-11 |

---

## 2. 需求描述

DeP2P 定义统一的消息格式，采用 Protobuf 序列化，支持消息头、长度前缀和扩展字段。

---

## 3. 背景与动机

### 3.1 问题陈述

P2P 消息格式需要解决：

1. **序列化效率**：二进制序列化
2. **版本兼容**：字段增删兼容
3. **边界识别**：消息分帧

### 3.2 目标

设计高效、可扩展的消息格式：
- 高效二进制序列化
- 向后兼容
- 清晰的消息边界

### 3.3 竞品参考

| 产品 | 消息格式 | 特点 |
|------|----------|------|
| **iroh** | 应用自定义 | 灵活 |
| **go-libp2p** | Protobuf + varint | 成熟 |
| **torrent** | 固定二进制 | 简单 |

**选择**：Protobuf + varint 长度前缀

---

## 4. 需求详情

### 4.1 功能要求

1. **Protobuf 序列化**：高效、可扩展
2. **长度前缀**：消息边界
3. **消息头**：类型、版本、标志
4. **请求-响应关联**：请求 ID

### 4.2 消息结构

```
+----------------+----------------+----------------+
|  Length (varint)               |  Message       |
+----------------+----------------+----------------+
```

### 4.3 消息头

```protobuf
// message.proto

syntax = "proto3";

package dep2p.protocol;

// MessageHeader 消息头
message MessageHeader {
    // 消息类型
    MessageType type = 1;
    
    // 请求 ID（用于请求-响应关联）
    uint64 request_id = 2;
    
    // 标志位
    uint32 flags = 3;
    
    // 协议版本
    string protocol_version = 4;
}

// MessageType 消息类型
enum MessageType {
    MESSAGE_TYPE_UNSPECIFIED = 0;
    MESSAGE_TYPE_REQUEST = 1;
    MESSAGE_TYPE_RESPONSE = 2;
    MESSAGE_TYPE_NOTIFICATION = 3;
    MESSAGE_TYPE_ERROR = 4;
}
```

### 4.4 消息封装

```protobuf
// Envelope 消息封装
message Envelope {
    // 消息头
    MessageHeader header = 1;
    
    // 消息体（应用数据）
    bytes payload = 2;
}
```

### 4.5 编解码接口

```go
// MessageCodec 消息编解码器
type MessageCodec interface {
    // Encode 编码消息
    Encode(msg *Envelope) ([]byte, error)
    
    // Decode 解码消息
    Decode(data []byte) (*Envelope, error)
    
    // EncodeWithLength 编码带长度前缀的消息
    EncodeWithLength(msg *Envelope) ([]byte, error)
    
    // DecodeWithLength 解码带长度前缀的消息
    DecodeWithLength(r io.Reader) (*Envelope, error)
}

// 默认实现
var DefaultCodec = &ProtobufCodec{}
```

### 4.6 长度前缀

```go
// 使用 varint 编码长度
// 最大消息长度：2^32 - 1 字节（4GB）

// WriteLength 写入长度前缀
func WriteLength(w io.Writer, length int) error {
    buf := make([]byte, binary.MaxVarintLen64)
    n := binary.PutUvarint(buf, uint64(length))
    _, err := w.Write(buf[:n])
    return err
}

// ReadLength 读取长度前缀
func ReadLength(r io.Reader) (int, error) {
    return binary.ReadUvarint(byteReader{r})
}
```

### 4.7 请求-响应模式

```go
// 请求消息
request := &Envelope{
    Header: &MessageHeader{
        Type:      MessageType_REQUEST,
        RequestId: generateRequestID(),
    },
    Payload: requestData,
}

// 响应消息（关联 RequestId）
response := &Envelope{
    Header: &MessageHeader{
        Type:      MessageType_RESPONSE,
        RequestId: request.Header.RequestId,
    },
    Payload: responseData,
}
```

### 4.8 错误消息

```protobuf
// ErrorMessage 错误消息
message ErrorMessage {
    // 错误码
    int32 code = 1;
    
    // 错误描述
    string message = 2;
    
    // 详细信息（可选）
    map<string, string> details = 3;
}
```

### 4.9 使用示例

```go
// 编码
msg := &Envelope{
    Header: &MessageHeader{
        Type:      MessageType_REQUEST,
        RequestId: 12345,
    },
    Payload: []byte("hello"),
}

data, _ := codec.EncodeWithLength(msg)
stream.Write(data)

// 解码
msg, _ := codec.DecodeWithLength(stream)
fmt.Printf("Type: %v, Payload: %s\n", msg.Header.Type, msg.Payload)
```

### 4.10 限制

| 限制 | 值 | 说明 |
|------|---|------|
| 最大消息长度 | 16 MB | 可配置 |
| 最大头部长度 | 1 KB | 固定 |
| RequestId 范围 | uint64 | 随机生成 |

### 4.11 错误处理

| 场景 | 错误 | 说明 |
|------|------|------|
| 消息过大 | `ErrMessageTooLarge` | 超过限制 |
| 解码失败 | `ErrDecodeFailure` | Protobuf 错误 |
| 无效消息类型 | `ErrInvalidMessageType` | 未知类型 |
| 头部无效 | `ErrInvalidHeader` | 头部格式错误 |

---

## 5. 验收标准

- [ ] Protobuf 序列化正确
- [ ] varint 长度前缀正确
- [ ] 消息头包含必要字段
- [ ] 请求-响应正确关联
- [ ] 错误消息格式正确
- [ ] 大消息正确处理
- [ ] 错误场景正确处理

---

## 6. 非功能要求

| 维度 | 要求 |
|------|------|
| **性能** | 编解码 < 1ms (1KB) |
| **空间** | 头部开销 < 50 bytes |
| **兼容** | 向后兼容 Protobuf |

---

## 7. 关联文档

| 类型 | 链接 |
|------|------|
| **竞品** | [协议设计对比](../../references/comparison/protocol/01-protocol-design.md) |
| **需求** | [REQ-PROTO-001](REQ-PROTO-001.md): 协议命名空间 |
| **需求** | [REQ-PROTO-003](REQ-PROTO-003.md): 流式通信 |

---

## 8. 实现追踪

### 8.1 代码引用

| 文件 | 符号 | 状态 |
|------|------|------|
| `pkg/protocol/message.proto` | `Envelope` | ⏳ 待实现 |
| `internal/core/protocol/codec.go` | `MessageCodec` | ⏳ 待实现 |

### 8.2 测试证据

| 测试文件 | 测试函数 | 状态 |
|----------|----------|------|
| `internal/core/protocol/codec_test.go` | `TestEncode` | ⏳ 待实现 |
| `internal/core/protocol/codec_test.go` | `TestDecode` | ⏳ 待实现 |

---

## 9. 变更历史

| 日期 | 版本 | 变更说明 |
|------|------|----------|
| 2026-01-11 | 1.0 | 初始版本 |
