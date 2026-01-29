# 测试夹具 (Fixtures)

> 测试数据、模拟对象、测试配置

---

## 概述

测试夹具是测试所需的预定义数据和配置，用于创建可重复的测试环境。

---

## 夹具类型

### 1. 密钥夹具

用于测试身份相关功能的预生成密钥。

```
fixtures/
└── keys/
    ├── test_key_1.pem      # 测试私钥 1
    ├── test_key_1.pub      # 测试公钥 1
    ├── test_key_2.pem      # 测试私钥 2
    └── test_key_2.pub      # 测试公钥 2
```

**使用示例**：

```go
func loadTestKey(t *testing.T, name string) ed25519.PrivateKey {
    t.Helper()
    data, err := os.ReadFile(filepath.Join("fixtures", "keys", name+".pem"))
    require.NoError(t, err)
    // 解析并返回密钥
}
```

**注意**：测试密钥仅用于测试，不得用于生产环境。

---

### 2. 证书夹具

用于测试 TLS 相关功能的预生成证书。

```
fixtures/
└── certs/
    ├── ca.crt              # 测试 CA 证书
    ├── ca.key              # 测试 CA 私钥
    ├── server.crt          # 服务器证书
    ├── server.key          # 服务器私钥
    └── expired.crt         # 过期证书（用于测试）
```

---

### 3. 配置夹具

预定义的配置文件用于不同测试场景。

```
fixtures/
└── configs/
    ├── default.yaml        # 默认配置
    ├── relay.yaml          # Relay 节点配置
    ├── minimal.yaml        # 最小配置
    └── stress.yaml         # 压力测试配置
```

**配置示例**：

```yaml
# fixtures/configs/default.yaml
node:
  listen_addrs:
    - /ip4/127.0.0.1/tcp/0
  connection_manager:
    low_watermark: 10
    high_watermark: 100

relay:
  enabled: true
  hop_limit: 2
```

---

### 4. 数据夹具

预定义的测试数据。

```
fixtures/
└── data/
    ├── messages/           # 测试消息
    │   ├── small.bin       # 小消息 (1KB)
    │   ├── medium.bin      # 中等消息 (100KB)
    │   └── large.bin       # 大消息 (10MB)
    └── protocols/          # 协议数据
        ├── handshake.bin   # 握手数据
        └── invalid.bin     # 无效数据（用于错误测试）
```

---

## 夹具管理

### 生成脚本

```bash
#!/bin/bash
# scripts/generate-fixtures.sh

# 生成测试密钥
for i in 1 2 3; do
    openssl genpkey -algorithm ed25519 -out fixtures/keys/test_key_$i.pem
    openssl pkey -in fixtures/keys/test_key_$i.pem -pubout -out fixtures/keys/test_key_$i.pub
done

# 生成测试证书
openssl req -x509 -newkey rsa:2048 -keyout fixtures/certs/ca.key \
    -out fixtures/certs/ca.crt -days 365 -nodes -subj "/CN=Test CA"

# 生成测试数据
dd if=/dev/urandom of=fixtures/data/messages/small.bin bs=1K count=1
dd if=/dev/urandom of=fixtures/data/messages/medium.bin bs=1K count=100
dd if=/dev/urandom of=fixtures/data/messages/large.bin bs=1M count=10
```

### 加载辅助函数

```go
// testutil/fixtures.go

package testutil

import (
    "os"
    "path/filepath"
    "testing"
)

const fixturesDir = "fixtures"

// LoadFixture 加载测试夹具文件
func LoadFixture(t *testing.T, path string) []byte {
    t.Helper()
    data, err := os.ReadFile(filepath.Join(fixturesDir, path))
    if err != nil {
        t.Fatalf("failed to load fixture %s: %v", path, err)
    }
    return data
}

// LoadConfig 加载测试配置
func LoadConfig(t *testing.T, name string) *Config {
    t.Helper()
    data := LoadFixture(t, filepath.Join("configs", name+".yaml"))
    // 解析配置
    return config
}

// LoadTestKey 加载测试密钥
func LoadTestKey(t *testing.T, name string) ed25519.PrivateKey {
    t.Helper()
    data := LoadFixture(t, filepath.Join("keys", name+".pem"))
    // 解析密钥
    return key
}
```

---

## 动态夹具

对于需要动态生成的夹具，使用工厂函数。

```go
// testutil/factories.go

// NodeFactory 创建测试节点
type NodeFactory struct {
    t      *testing.T
    config *Config
}

func NewNodeFactory(t *testing.T) *NodeFactory {
    return &NodeFactory{t: t, config: DefaultConfig()}
}

func (f *NodeFactory) WithConfig(c *Config) *NodeFactory {
    f.config = c
    return f
}

func (f *NodeFactory) Create() *Node {
    node, err := NewNode(f.config)
    require.NoError(f.t, err)
    f.t.Cleanup(func() { node.Close() })
    return node
}

// 使用示例
func TestExample(t *testing.T) {
    node := NewNodeFactory(t).
        WithConfig(LoadConfig(t, "relay")).
        Create()
    // ...
}
```

---

## 夹具版本管理

### 版本记录

```
fixtures/
└── VERSION        # 夹具版本号
```

```
# fixtures/VERSION
1.0.0
```

### 兼容性检查

```go
func checkFixturesVersion(t *testing.T) {
    t.Helper()
    data, err := os.ReadFile("fixtures/VERSION")
    require.NoError(t, err)
    version := strings.TrimSpace(string(data))
    if version != expectedVersion {
        t.Fatalf("fixtures version mismatch: expected %s, got %s", expectedVersion, version)
    }
}
```

---

## 最佳实践

| 实践 | 说明 |
|------|------|
| **最小化** | 只包含必要的夹具数据 |
| **版本化** | 夹具随代码版本控制 |
| **文档化** | 说明每个夹具的用途 |
| **安全性** | 不包含真实的密钥或敏感数据 |
| **可重现** | 提供生成脚本 |

---

**最后更新**：2026-01-11
