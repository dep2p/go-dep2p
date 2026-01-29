# 快速开始 (Getting Started)

> 环境搭建、首次贡献

---

## 目录结构

```
getting_started/
├── README.md              # 本文件
├── setup.md               # 环境搭建
└── first_contribution.md  # 第一次贡献
```

---

## 概述

本目录帮助新开发者快速上手 DeP2P 开发。

---

## 快速开始

### 1. 环境准备

```bash
# 确认 Go 版本
go version  # 需要 1.21+

# 克隆仓库
git clone https://github.com/dep2p/dep2p.git
cd dep2p
```

### 2. 运行测试

```bash
# 运行所有测试
go test ./...

# 运行带覆盖率
go test -cover ./...
```

### 3. 构建

```bash
# 构建主程序
go build -o bin/dep2p ./cmd/dep2p

# 运行
./bin/dep2p --help
```

---

## 快速链接

| 文档 | 说明 |
|------|------|
| [setup.md](setup.md) | 详细环境搭建指南 |
| [first_contribution.md](first_contribution.md) | 第一次贡献指南 |

---

**最后更新**：2026-01-11
