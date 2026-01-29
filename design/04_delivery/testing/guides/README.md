# 测试执行指南 (Guides)

本目录包含测试执行相关的指南文档。

---

## 文档列表

| 文档 | 描述 | 受众 |
|------|------|------|
| [`execution_guide.md`](./execution_guide.md) | 测试执行指南 | 所有开发者 |

---

## 内容概览

### execution_guide.md

完整的测试执行指南，包括：

1. **快速开始**
   - 日常开发测试命令
   - 提交前检查
   - 发布前验证

2. **测试分层执行**
   - 单元测试（Layer 1）
   - 集成测试（Layer 2）
   - E2E 测试（Layer 3）

3. **高级测试选项**
   - 覆盖率分析
   - 并行测试
   - 测试筛选
   - 详细输出

4. **常见任务**
   - 调试失败的测试
   - 检查数据竞争
   - 性能测试
   - 测试覆盖率提升

5. **CI/CD 集成**
   - GitHub Actions 示例
   - Makefile 快捷命令

6. **测试脚本**
   - 测试基线脚本
   - 模块测试脚本

7. **故障排查**
   - 测试失败
   - Race 检测问题
   - 覆盖率问题

8. **最佳实践**
   - 测试原则
   - 命令速查

---

## 快速命令

### 日常开发

```bash
# 快速测试
go test ./... -short

# 完整单元测试
go test ./...
```

### Race 检测

```bash
# 快速 Race 检测
go test -race ./... -short

# 完整 Race 检测
go test -race ./...
```

### 集成测试

```bash
# 所有集成测试
go test -tags=integration ./tests/integration/...
```

### E2E 测试

```bash
# 所有 E2E 测试
go test -tags=e2e ./tests/e2e/...
```

---

## 相关文档

- [`../overview.md`](../overview.md) - 测试策略改进概览
- [`../strategy/quality_gates.md`](../strategy/quality_gates.md) - 质量门禁
- [`../implementation/plan.md`](../implementation/plan.md) - 实施计划

---

**最后更新**: 2026-01-21
