# 📚 DeP2P Documentation

Welcome to DeP2P documentation center! Please select your preferred language.

---

## 🌍 Language Selection / 语言选择

- **[🇨🇳 中文 (Chinese)](zh/)** - 简体中文文档
- **[🇺🇸 English](en/)** - English documentation

---

## 📖 Documentation Structure

DeP2P documentation is organized in a layered structure:

```
docs/
├── zh/                    # 中文文档 (Chinese)
│   ├── getting-started/   # 用户入门
│   ├── concepts/          # 概念解释
│   ├── tutorials/         # 教程
│   ├── how-to/            # 操作指南
│   ├── reference/         # API 参考
│   └── contributing/      # 贡献指南
│
└── en/                    # English documentation
    ├── getting-started/   # Getting Started
    ├── concepts/          # Concepts
    ├── tutorials/         # Tutorials
    ├── how-to/            # How-To Guides
    ├── reference/         # API Reference
    └── contributing/      # Contributing
```

---

## 🚀 Quick Start

### For Chinese Users / 中文用户

👉 **[进入中文文档](zh/)**

**推荐阅读路径**：
1. [Hello World](zh/tutorials/01-hello-world.md) - 5 分钟启动第一个节点
2. [局域网聊天](zh/tutorials/02-local-chat.md) - mDNS + Realm 成员管理
3. [核心概念](zh/concepts/core-concepts.md) - 身份优先、Realm 隔离

### For English Users

👉 **[Go to English Documentation](en/)**

---

## 📝 Documentation Status

| Language | Status | Coverage |
|----------|--------|----------|
| 🇨🇳 中文 | ✅ Complete | 100% |
| 🇺🇸 English | 🚧 In Progress | Coming soon |

---

## 🔧 Configuration

DeP2P 配置说明：

- **[配置指南](configuration.md)** - 完整配置参考（预设、连接性、断开检测等）

### 快速配置示例

```go
// 桌面端默认配置
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
node.Start(ctx)

// 云服务器配置
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithTrustSTUNAddresses(true),
    dep2p.WithKnownPeers(knownPeers),
)
node.Start(ctx)
```

---

## 🤝 Contributing Translations

We welcome contributions to improve documentation translations! Please see:

- [Contributing Guide (中文)](zh/contributing/README.md)
- [Contributing Guide (English)](en/contributing/README.md)

---

## 🔗 Related Resources

- **Design Documents**: See [design/](../design/README.md) - 架构决策记录（ADR）、协议约束、组件设计
- **Examples**: See [examples/](../examples/) - 代码示例
- **Configuration**: See [configuration.md](configuration.md) - 配置指南
