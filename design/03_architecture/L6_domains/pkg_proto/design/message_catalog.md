# pkg_proto 消息目录

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 消息分类索引

| 类别 | proto 文件 | 消息数 | 用途 |
|------|-----------|--------|------|
| [公共消息](#1-公共消息) | common.proto | 5 | 基础类型 |
| [密钥消息](#2-密钥消息) | key.proto | 4 | 密钥序列化 |
| [节点消息](#3-节点消息) | peer.proto | 5 | 节点记录 |
| [身份识别](#4-身份识别) | identify.proto | 2 | 节点信息交换 |
| [AutoNAT](#5-autonat) | autonat.proto | 4 | NAT 类型检测 |
| [打洞](#6-holepunch) | holepunch.proto | 1 | NAT 穿透 |
| [中继](#7-relay) | relay.proto, voucher.proto | 6 | 中继服务 |
| [DHT](#8-dht) | dht.proto | 3 | 分布式哈希表 |
| [Rendezvous](#9-rendezvous) | rendezvous.proto | 6 | 命名空间发现 |
| [Realm](#10-realm) | realm.proto | 6 | Realm 认证 |
| [Messaging](#11-messaging) | messaging.proto | 4 | 消息传递 |
| [GossipSub](#12-gossipsub) | gossipsub.proto | 11 | 发布订阅 |

**总计**：13 个 proto 文件，45+ 消息类型

---

## 1. 公共消息（common.proto）

### Timestamp
```protobuf
message Timestamp {
  int64 seconds = 1;
  int32 nanos = 2;
}
```
**用途**：时间戳表示

### Error
```protobuf
message Error {
  int32 code = 1;
  string message = 2;
  map<string, string> details = 3;
}
```
**用途**：错误信息传递

### Result
```protobuf
message Result {
  bool success = 1;
  Error error = 2;
}
```
**用途**：通用操作结果

### Version
```protobuf
message Version {
  int32 major = 1;
  int32 minor = 2;
  int32 patch = 3;
}
```
**用途**：版本信息

---

## 2. 密钥消息（key.proto）

### KeyType（枚举）
```protobuf
enum KeyType {
  KEY_TYPE_UNSPECIFIED = 0;
  RSA = 1;
  Ed25519 = 2;
  Secp256k1 = 3;
  ECDSA = 4;
}
```

### PublicKey
```protobuf
message PublicKey {
  KeyType type = 1;
  bytes data = 2;
}
```
**用途**：公钥序列化

### PrivateKey
```protobuf
message PrivateKey {
  KeyType type = 1;
  bytes data = 2;
}
```
**用途**：私钥序列化

### Signature
```protobuf
message Signature {
  KeyType type = 1;
  bytes data = 2;
}
```
**用途**：签名数据

---

## 3. 节点消息（peer.proto）

### PeerRecord
```protobuf
message PeerRecord {
  bytes peer_id = 1;
  uint64 seq = 2;
  repeated AddressInfo addresses = 3;
}
```
**用途**：节点记录（DHT 存储）

### SignedPeerRecord
```protobuf
message SignedPeerRecord {
  bytes peer_id = 1;
  bytes public_key = 2;
  bytes payload = 3;
  bytes signature = 4;
}
```
**用途**：签名的节点记录（防伪造）

---

## 4. 身份识别（identify.proto）

### Identify
```protobuf
message Identify {
  bytes protocol_version = 1;
  bytes agent_version = 2;
  bytes public_key = 3;
  repeated bytes listen_addrs = 4;
  bytes observed_addr = 5;
  repeated string protocols = 6;
  bytes signed_peer_record = 8;
}
```
**用途**：节点身份交换（连接建立后）

**协议**：`/dep2p/sys/identify/1.0.0`

---

## 5. AutoNAT（autonat.proto）

### Message
```protobuf
enum MessageType {
  DIAL = 0;
  DIAL_RESPONSE = 1;
}

message Message {
  MessageType type = 1;
  Dial dial = 2;
  DialResponse dial_response = 3;
}
```
**用途**：NAT 类型自动检测

**协议**：`/dep2p/sys/autonat/1.0.0`

---

## 6. Holepunch（holepunch.proto）

### HolePunch
```protobuf
enum Type {
  CONNECT = 0;
  SYNC = 1;
}

message HolePunch {
  Type type = 1;
  repeated bytes obs_addrs = 2;
}
```
**用途**：NAT 打洞（DCUtR）

**协议**：`/dep2p/sys/holepunch/1.0.0`

---

## 7. Relay（relay.proto）

### HopMessage
```protobuf
message HopMessage {
  enum Type {
    RESERVE = 0;
    CONNECT = 1;
    STATUS = 2;
  }
  
  Type type = 1;
  Peer peer = 2;
  Reservation reservation = 3;
  // ...
}
```
**用途**：中继预留和连接

**协议**：`/dep2p/relay/1.0.0/hop`, `/dep2p/relay/1.0.0/stop`

---

## 8. DHT（dht.proto）

### Message
```protobuf
enum MessageType {
  PUT_VALUE = 0;
  GET_VALUE = 1;
  FIND_NODE = 4;
  // ...
}

message Message {
  MessageType type = 1;
  bytes key = 2;
  bytes value = 3;
  repeated Peer closer = 10;
}
```
**用途**：Kademlia DHT 操作

**协议**：`/dep2p/sys/dht/1.0.0`

---

## 9. Rendezvous（rendezvous.proto）

### Message
```protobuf
message Message {
  enum MessageType {
    REGISTER = 0;
    DISCOVER = 3;
    // ...
  }
  
  MessageType type = 1;
  Register register = 2;
  Discover discover = 5;
  // ...
}
```
**用途**：命名空间发现

**协议**：`/dep2p/sys/rendezvous/1.0.0`

---

## 10. Realm（realm.proto）- DeP2P 特有

### AuthRequest
```protobuf
message AuthRequest {
  bytes realm_id = 1;
  bytes peer_id = 2;
  bytes challenge_response = 3;
  uint64 timestamp = 4;
  bytes nonce = 5;
}
```
**用途**：Realm 成员认证

**协议**：`/dep2p/realm/<realmID>/auth/1.0.0`

### MemberList
```protobuf
message MemberList {
  bytes realm_id = 1;
  repeated MemberInfo members = 2;
  uint64 version = 3;
}
```
**用途**：Realm 成员列表同步

---

## 11. Messaging（messaging.proto）- DeP2P 特有

### Message
```protobuf
enum MessageType {
  DIRECT = 0;
  BROADCAST = 1;
  ACK = 2;
}

message Message {
  bytes id = 1;
  bytes from = 2;
  bytes to = 3;
  string topic = 4;
  MessageType type = 5;
  bytes payload = 7;
  // ...
}
```
**用途**：应用层消息传递

**协议**：`/dep2p/app/<realmID>/messaging/1.0.0`

---

## 12. GossipSub（gossipsub.proto）

### RPC
```protobuf
message RPC {
  repeated SubOpts subscriptions = 1;
  repeated Message publish = 2;
  ControlMessage control = 3;
}
```
**用途**：GossipSub 发布订阅

**协议**：`/dep2p/app/<realmID>/pubsub/1.0.0`

---

## 字段编号分配表

### common.proto
| 消息 | 字段 | 编号 | 类型 |
|------|------|------|------|
| Timestamp | seconds | 1 | int64 |
| | nanos | 2 | int32 |
| Error | code | 1 | int32 |
| | message | 2 | string |
| | details | 3 | map |

### key.proto
| 消息 | 字段 | 编号 | 类型 |
|------|------|------|------|
| PublicKey | type | 1 | KeyType |
| | data | 2 | bytes |
| PrivateKey | type | 1 | KeyType |
| | data | 2 | bytes |

### realm.proto（DeP2P 特有）
| 消息 | 字段 | 编号 | 类型 |
|------|------|------|------|
| AuthRequest | realm_id | 1 | bytes |
| | peer_id | 2 | bytes |
| | challenge_response | 3 | bytes |
| | timestamp | 4 | uint64 |
| | nonce | 5 | bytes |

---

## 相关文档

- [overview.md](overview.md) - 设计概述
- [../coding/guidelines.md](../coding/guidelines.md) - 编码指南

---

**最后更新**：2026-01-13
