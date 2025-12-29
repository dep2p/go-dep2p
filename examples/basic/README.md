# 基础示例 - QuickStart 快速开始

这是 DeP2P 最简单的示例，展示如何用最少的代码创建一个 P2P 节点。

## 你将学到什么

- ✅ 如何创建一个 P2P 节点
- ✅ 什么是节点 ID
- ✅ 如何监听连接
- ✅ 如何注册协议处理器
- ✅ 如何优雅地关闭节点

## 前置要求

- **Go 版本**: 1.21+
- **操作系统**: Linux, macOS, Windows
- **网络**: 不需要（本示例仅启动节点，不建立连接）

### 检查 Go 环境

```bash
# 检查 Go 版本
go version
# 应该显示: go version go1.21.x 或更高

# 检查 Go 环境
go env GOPATH
# 确保已设置 GOPATH
```

## 快速开始

```bash
# 1. 进入示例目录
cd examples/basic/

# 2. 运行示例
go run main.go

# 3. 按 Ctrl+C 停止
```

就这么简单！你已经运行了第一个 P2P 节点。

## 预期输出

运行后你会看到类似的输出：

```
=== DeP2P 简单示例 ===

正在创建 dep2p 节点...
节点 ID: 5Q2STWvBExampleNodeID...

监听地址:
  [1] /ip4/127.0.0.1/udp/54321/quic-v1

已注册 /echo/1.0.0 协议处理器

=== 节点已启动 ===
按 Ctrl+C 退出
```

### 输出解释

让我们逐行理解输出的含义：

#### 节点 ID
```
节点 ID: 5Q2STWvBExampleNodeID...
```
- **节点 ID** 是节点的唯一标识符，类似"身份证号"
- 基于节点的公钥生成，全网唯一
- dep2p 的 NodeID 外部表示为 **Base58** 字符串（如 `5Q2STWvB...`）
- 其他节点可以用这个 ID 来连接你

💡 **类比**: 就像你的手机号，别人知道你的号码才能给你打电话。

#### 监听地址
```
监听地址:
  [1] /ip4/127.0.0.1/udp/54321/quic-v1
```
- **监听地址** 是节点在网络上的"门牌号"
- `/ip4/127.0.0.1` = IPv4 地址 127.0.0.1（本机回环）
- `/udp/54321` = UDP 端口 54321（随机分配）
- `/quic-v1` = 使用 QUIC 协议版本 1

💡 **类比**: 就像你家的地址，别人知道地址才能找到你家。

#### 协议处理器
```
已注册 /echo/1.0.0 协议处理器
```
- **协议** 定义了节点能做什么
- `/echo/1.0.0` 是一个示例协议
- 其他节点可以用这个协议和你通信

💡 **类比**: 就像你会说的语言，别人说同样的语言才能和你交流。

## 代码详解

让我们逐段分析代码（查看 `main.go`）：

### 1. 创建上下文和信号处理

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

signalCh := make(chan os.Signal, 1)
signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
go func() {
    <-signalCh
    fmt.Println("\n收到中断信号，准备关闭...")
    cancel()
}()
```

**作用**: 
- 创建可取消的上下文，用于控制节点生命周期
- 监听 Ctrl+C 信号，让程序能优雅退出
- `cancel()` 会通知节点开始关闭流程

**为什么需要**: P2P 节点可能有多个连接和后台任务，需要优雅关闭而不是直接杀进程。

### 2. 快速创建节点

```go
endpoint, err := dep2p.QuickStart(ctx)
if err != nil {
    fmt.Printf("创建节点失败: %v\n", err)
    os.Exit(1)
}
defer endpoint.Close()
```

**作用**:
- `QuickStart` 是最简单的启动方式，使用默认配置
- 自动选择随机端口，自动启动监听
- `defer endpoint.Close()` 确保程序退出时关闭节点

**QuickStart 做了什么**:
1. 生成身份密钥对
2. 创建节点配置（使用默认值）
3. 启动网络监听
4. 启动后台服务（连接管理、心跳等）

### 3. 打印节点信息

```go
fmt.Printf("节点 ID: %s\n", endpoint.ID())

addrs := endpoint.ListenAddrs()
if len(addrs) > 0 {
    fmt.Println("监听地址:")
    for i, addr := range addrs {
        fmt.Printf("  [%d] %s\n", i+1, addr)
    }
}
```

**作用**:
- `endpoint.ID()` 返回节点 ID
- `endpoint.ListenAddrs()` 返回所有监听地址
- 这些信息可以分享给其他节点，让他们连接你

### 4. 注册协议处理器

```go
endpoint.SetProtocolHandler("/echo/1.0.0", func(stream dep2p.Stream) {
    defer stream.Close()
    fmt.Println("收到新的流连接")
    // TODO: 实现 echo 逻辑
})
```

**作用**:
- 注册一个协议处理器
- 当其他节点用这个协议连接时，这个函数会被调用
- `stream` 是通信通道，类似 TCP 连接

**协议命名约定**:
- 格式: `/<应用名>/<功能>/<版本>`
- 例如: `/dep2p/echo/1.0.0`
- 版本号允许协议升级而不破坏兼容性

### 5. 等待退出

```go
<-ctx.Done()
fmt.Println("节点已关闭")
```

**作用**:
- 阻塞主线程，直到收到 Ctrl+C
- `ctx.Done()` 会在 `cancel()` 被调用时返回
- 然后执行清理工作（`defer` 语句）

## 常见问题

### Q: 端口 54321 被占用怎么办？

**A**: 不用担心！`QuickStart` 使用端口 0，系统会自动分配空闲端口。每次运行端口都可能不同。

如果你想指定端口：

```go
endpoint, err := dep2p.Start(ctx,
    dep2p.WithListenPort(3000),
)
```

### Q: 节点 ID 每次都不同？

**A**: 是的！默认情况下，每次运行都会生成新的密钥对，因此 ID 不同。

如果你想保持相同的 ID，需要保存并加载密钥：

```go
// 第一次运行时保存密钥
// 之后运行时加载密钥
// 详见 identity 模块文档
```

### Q: 为什么看不到其他节点？

**A**: 这个示例只启动节点，不主动连接其他节点。要实现连接，请看：
- [Echo 示例](../echo/) - 学习如何连接
- [Chat 示例](../chat/) - 学习自动发现

### Q: 127.0.0.1 是什么意思？

**A**: `127.0.0.1` 是本机回环地址，只能从本机访问。如果要让其他设备连接，使用 `WithListenPort`：

```go
dep2p.WithListenPort(0)
// 监听所有网络接口（IPv4 + IPv6），0 表示随机端口
```

### Q: 可以同时运行多个实例吗？

**A**: 可以！开多个终端运行即可。由于使用随机端口，不会冲突。

```bash
# 终端 1
go run main.go

# 终端 2
go run main.go

# 终端 3
go run main.go
```

### Q: 出现 "注意: dep2p 核心框架正在开发中" 怎么办？

**A**: 这是正常提示，表示某些高级功能还在完善中。基础功能已经可用。

## 故障排除

### 编译错误

```bash
# 错误: cannot find package
go mod download
go mod tidy

# 错误: go: module requires Go 1.21
# 升级 Go 版本到 1.21+
```

### 运行时错误

```bash
# 错误: bind: permission denied
# 某些系统需要权限才能绑定低端口（< 1024）
# 解决: 使用高端口或 sudo（不推荐）

# 错误: context canceled
# 这是正常现象，表示收到 Ctrl+C 正在关闭
```

### 网络问题

如果看到网络相关错误：

1. **检查防火墙**: 确保允许 UDP 流量
2. **检查网络**: 确保网络连接正常
3. **检查端口**: 确保端口未被占用

## 进阶练习

掌握了基础示例后，试试这些练习：

### 练习 1: 修改监听端口

修改代码，让节点监听特定端口：

```go
endpoint, err := dep2p.Start(ctx,
    dep2p.WithListenPort(4001),
)
```

### 练习 2: 添加多个协议

注册多个协议处理器：

```go
endpoint.SetProtocolHandler("/echo/1.0.0", echoHandler)
endpoint.SetProtocolHandler("/ping/1.0.0", pingHandler)
endpoint.SetProtocolHandler("/chat/1.0.0", chatHandler)
```

### 练习 3: 打印更多信息

在代码中添加：

```go
// 打印通告地址
advAddrs := endpoint.AdvertisedAddrs()
fmt.Printf("通告地址: %v\n", advAddrs)

// 打印连接数
fmt.Printf("连接数: %d\n", endpoint.ConnectionCount())
```

### 练习 4: 实现真正的 Echo

完善 echo 处理器，实现消息回显：

```go
endpoint.SetProtocolHandler("/echo/1.0.0", func(stream dep2p.Stream) {
    defer stream.Close()
    
    buf := make([]byte, 1024)
    n, err := stream.Read(buf)
    if err != nil {
        return
    }
    
    stream.Write(buf[:n])
})
```

## 下一步

完成基础示例后，继续学习：

1. **[Echo 示例](../echo/)** - 学习节点间通信
2. **[Chat 示例](../chat/)** - 学习自动发现
3. **[Relay 示例](../relay/)** - 学习 NAT 穿透

## 相关资源

- **API 文档**: [pkg/](../../pkg/)
- **设计文档**: [docs/01-design/](../../docs/01-design/)
- **测试用例**: [tests/e2e/](../../tests/e2e/)

---

🎉 **恭喜！** 你已经创建了第一个 P2P 节点。继续探索其他示例吧！

