# 开发指南 (Development)

> 编码、测试、调试指南

---

## 目录结构

```
development/
├── README.md              # 本文件
├── coding_guide.md        # 编码指南
├── testing_guide.md       # 测试指南
└── debugging_guide.md     # 调试指南
```

---

## 概述

本目录提供 DeP2P 开发的详细指南，帮助开发者编写高质量的代码。

---

## 快速链接

| 文档 | 说明 |
|------|------|
| [coding_guide.md](coding_guide.md) | 代码风格和最佳实践 |
| [testing_guide.md](testing_guide.md) | 测试编写指南 |
| [debugging_guide.md](debugging_guide.md) | 调试技巧 |

---

## 开发工作流

```mermaid
flowchart LR
    A[设计] --> B[编码]
    B --> C[测试]
    C --> D[评审]
    D --> E[合并]
```

---

## 常用命令

```bash
# 构建
go build ./...

# 测试
go test ./...

# Lint
golangci-lint run

# 格式化
go fmt ./...
goimports -w .

# 生成
go generate ./...
```

---

**最后更新**：2026-01-11
