# Protocol PubSub 测试策略

> **组件**: P6-02 protocol_pubsub  
> **测试**: 54个测试用例

---

## 测试结构

### 单元测试
- Service: 9个测试
- Topic: 7个测试
- Subscription: 4个测试
- Mesh: 10个测试
- Message: 9个测试
- Validator: 6个测试

### 集成测试
- Integration: 3个测试

### 并发测试
- Concurrent: 6个测试

---

## 测试执行

**规范**: 必须逐个运行

```bash
go test -v -run TestService_Join .
go test -v -run TestTopic_String .
...
```

---

## 覆盖率

- **当前**: 53.3%
- **目标**: 80%
- **说明**: 测试中禁用了heartbeat

---

**最后更新**: 2026-01-14
