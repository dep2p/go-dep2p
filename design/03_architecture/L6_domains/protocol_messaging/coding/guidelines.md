# core_messaging 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/messaging/
├── module.go           # Fx 模块
├── messaging.go        # 消息服务
├── pubsub.go           # PubSub
├── streams.go          # 流管理
├── validator.go        # 验证器
└── *_test.go           # 测试
```

---

## 消息使用规范

### 请求-响应模式

```
// 发送请求
response, err := realm.Messaging().Request(ctx, target, "/myapp/query", request)
if err != nil {
    return err
}
// 处理响应

// 注册请求处理器
realm.Messaging().OnRequest("/myapp/query", func(ctx context.Context, from types.NodeID, data []byte) ([]byte, error) {
    // 处理请求
    return response, nil
})
```

### 单向通知模式

```
// 发送通知
err := realm.Messaging().Send(ctx, target, "/myapp/notify", data)

// 注册通知处理器
realm.Messaging().OnMessage("/myapp/notify", func(ctx context.Context, from types.NodeID, data []byte) {
    // 处理通知
})
```

---

## PubSub 使用规范

### 发布消息

```
err := realm.PubSub().Publish(ctx, "events", eventData)
```

### 订阅主题

```
sub, err := realm.PubSub().Subscribe("events")
if err != nil {
    return err
}
defer sub.Cancel()

for {
    msg, err := sub.Next(ctx)
    if err != nil {
        return err
    }
    // 处理消息
}
```

---

## 协议命名规范

```
/dep2p/realm/<realmID>/msg/<proto>  - 消息协议
/dep2p/realm/<realmID>/pubsub/<topic>  - PubSub 主题
/dep2p/app/<app>/<proto>  - 应用协议
```

---

## 并发模式

### 并发处理消息

```
// 消息处理器在独立 goroutine 中执行
realm.Messaging().OnMessage("/myapp/notify", func(ctx context.Context, from types.NodeID, data []byte) {
    // 这里在独立 goroutine 中执行
})
```

---

**最后更新**：2026-01-11
