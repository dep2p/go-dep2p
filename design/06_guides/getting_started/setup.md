# 环境搭建

> 开发环境配置指南

---

## 1. 系统要求

### 1.1 硬件要求

| 资源 | 最低配置 | 推荐配置 |
|------|----------|----------|
| CPU | 2 核 | 4 核+ |
| 内存 | 4 GB | 8 GB+ |
| 磁盘 | 10 GB | 20 GB+ |

### 1.2 操作系统

| 系统 | 版本 | 支持状态 |
|------|------|:--------:|
| Linux | Ubuntu 20.04+ | ✅ |
| macOS | 12.0+ | ✅ |
| Windows | 10/11 | ✅ |

---

## 2. 软件安装

### 2.1 Go 安装

**Linux/macOS**:

```bash
# 下载 Go 1.21
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz

# 解压安装
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz

# 配置环境变量
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# 验证
go version
```

**macOS (Homebrew)**:

```bash
brew install go@1.21
```

**Windows**:

下载并运行 [Go 安装程序](https://go.dev/dl/)。

### 2.2 Git 安装

**Linux**:

```bash
sudo apt-get install git
```

**macOS**:

```bash
brew install git
```

**Windows**:

下载并安装 [Git for Windows](https://gitforwindows.org/)。

### 2.3 开发工具

```bash
# 代码格式化
go install golang.org/x/tools/cmd/goimports@latest

# 静态分析
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Mock 生成
go install github.com/golang/mock/mockgen@latest

# 调试器
go install github.com/go-delve/delve/cmd/dlv@latest
```

---

## 3. 仓库设置

### 3.1 克隆仓库

```bash
# 克隆
git clone https://github.com/dep2p/dep2p.git
cd dep2p

# 或使用 SSH
git clone git@github.com:dep2p/dep2p.git
```

### 3.2 安装依赖

```bash
# 下载依赖
go mod download

# 验证依赖
go mod verify
```

### 3.3 验证环境

```bash
# 运行测试
go test ./...

# 构建
go build ./...

# 运行 lint
golangci-lint run
```

---

## 4. IDE 配置

### 4.1 VS Code

**推荐扩展**:

- Go (官方扩展)
- GitLens
- Error Lens

**settings.json**:

```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"],
  "editor.formatOnSave": true,
  "[go]": {
    "editor.defaultFormatter": "golang.go"
  }
}
```

### 4.2 GoLand

1. 打开项目目录
2. 确认 GOROOT 配置正确
3. 启用 Go Modules

### 4.3 Vim/Neovim

使用 [vim-go](https://github.com/fatih/vim-go) 插件。

---

## 5. 常见问题

### 5.1 Go Modules 问题

```bash
# 清理缓存
go clean -modcache

# 重新下载
go mod download
```

### 5.2 权限问题

```bash
# Linux/macOS
chmod +x scripts/*.sh
```

### 5.3 网络问题

```bash
# 设置 Go 代理
go env -w GOPROXY=https://goproxy.cn,direct
```

---

## 6. 下一步

- [第一次贡献](first_contribution.md)
- [编码指南](../development/coding_guide.md)
- [测试指南](../development/testing_guide.md)

---

**最后更新**：2026-01-11
