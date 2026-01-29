# 开发指南 (Guides)

> 开发者指南：如何开发、调试、贡献

---

## 目录结构

```
06_guides/
├── README.md              # 本文件
├── getting_started/       # 快速开始
│   ├── README.md
│   ├── setup.md           # 环境搭建
│   └── first_contribution.md # 第一次贡献
├── development/           # 开发指南
│   ├── README.md
│   ├── coding_guide.md    # 编码指南
│   ├── testing_guide.md   # 测试指南
│   └── debugging_guide.md # 调试指南
├── architecture/          # 架构指南
│   ├── README.md
│   └── module_guide.md    # 模块开发指南
└── operations/            # 运维指南
    ├── README.md
    └── deployment.md      # 部署指南
```

---

## 快速开始

### 环境要求

| 工具 | 版本 | 说明 |
|------|------|------|
| Go | ≥ 1.21 | 编程语言 |
| Git | ≥ 2.0 | 版本控制 |
| Make | - | 构建工具 |

### 克隆仓库

```bash
git clone https://github.com/dep2p/dep2p.git
cd dep2p
```

### 运行测试

```bash
# 单元测试
go test ./...

# 集成测试
go test ./tests/...

# 覆盖率
go test -cover ./...
```

### 构建

```bash
# 构建主程序
go build -o bin/dep2p ./cmd/dep2p

# 构建 Relay 服务器
go build -o bin/relay-server ./cmd/relay-server
```

---

## 开发工作流

### 分支策略

```
main        ← 稳定版本
  │
  └── develop  ← 开发分支
        │
        ├── feature/xxx  ← 功能分支
        ├── fix/xxx      ← 修复分支
        └── refactor/xxx ← 重构分支
```

### 提交规范

```
<type>(<scope>): <subject>

<body>

<footer>
```

类型：
- `feat`: 新功能
- `fix`: 修复
- `docs`: 文档
- `style`: 格式
- `refactor`: 重构
- `test`: 测试
- `chore`: 杂项

---

## 调试技巧

### 日志级别

```go
// 设置日志级别
dep2p.WithLogLevel("debug")
```

### 常用调试工具

| 工具 | 用途 |
|------|------|
| `dlv` | Go 调试器 |
| `pprof` | 性能分析 |
| `tcpdump` | 网络抓包 |

---

## 快速链接

| 文档 | 说明 |
|------|------|
| [getting_started/](getting_started/) | 快速开始 |
| [development/](development/) | 开发指南 |
| [architecture/](architecture/) | 架构指南 |
| [operations/](operations/) | 运维指南 |

---

**最后更新**：2026-01-11
