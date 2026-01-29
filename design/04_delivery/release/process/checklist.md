# 发布检查清单

> 发布前、发布中、发布后的完整检查项

---

## 元信息

| 字段 | 值 |
|------|-----|
| **状态** | approved |
| **Owner** | DeP2P Team |
| **创建日期** | 2026-01-11 |
| **更新日期** | 2026-01-11 |

---

## 1. 发布前准备

### 1.1 代码准备

- [ ] 所有计划功能已合并到发布分支
- [ ] 所有已知阻断问题已修复
- [ ] 代码冻结（feature freeze）
- [ ] 最后一次代码审查完成

### 1.2 版本准备

- [ ] 更新 `version.json`
- [ ] 更新 `CHANGELOG.md`
- [ ] 更新 API 文档
- [ ] 更新用户指南（如需要）

### 1.3 测试验证

- [ ] 所有单元测试通过
- [ ] 所有集成测试通过
- [ ] E2E 测试通过
- [ ] 基准测试无回归（> 10%）
- [ ] 安全扫描通过
- [ ] 覆盖率不低于阈值

---

## 2. 发布执行

### 2.1 创建发布

```bash
# 1. 确保在正确的分支
git checkout main
git pull origin main

# 2. 创建发布标签
git tag -a v1.0.0 -m "Release v1.0.0"

# 3. 推送标签
git push origin v1.0.0
```

### 2.2 CI/CD 检查

- [ ] 发布流水线触发
- [ ] 构建成功
- [ ] 所有测试通过
- [ ] 安全扫描通过
- [ ] 产物生成成功

### 2.3 发布产物

- [ ] GitHub Release 创建
- [ ] 发布说明生成
- [ ] 二进制文件上传
- [ ] Docker 镜像推送（如适用）

---

## 3. 发布验证

### 3.1 功能验证

- [ ] 从 GitHub 下载并验证
- [ ] 基本功能测试通过
- [ ] 示例代码可运行
- [ ] 文档链接有效

### 3.2 兼容性验证

- [ ] 从旧版本升级测试
- [ ] 配置兼容性验证
- [ ] API 兼容性验证

### 3.3 发布公告

- [ ] 更新项目主页
- [ ] 发送发布公告（如适用）
- [ ] 更新相关文档

---

## 4. 发布后

### 4.1 监控

- [ ] 监控 Issue 反馈
- [ ] 监控下载/使用情况
- [ ] 准备快速响应

### 4.2 文档

- [ ] 归档发布记录
- [ ] 更新版本历史
- [ ] 记录发布问题（如有）

---

## 5. 紧急发布（热修复）

### 5.1 触发条件

- 安全漏洞
- 严重功能 Bug
- 数据损坏风险

### 5.2 简化流程

- [ ] 创建热修复分支
- [ ] 修复并测试
- [ ] 紧急代码审查（至少 1 人）
- [ ] 快速测试通过
- [ ] 发布并通知

### 5.3 后续

- [ ] 合并到主分支
- [ ] 补充完整测试
- [ ] 编写事后总结

---

## 6. 检查清单模板

### 6.1 发布准备表

```markdown
## 发布准备检查 - v{VERSION}

**发布负责人**: @xxx
**计划发布日期**: YYYY-MM-DD

### 代码准备
- [ ] 功能完成
- [ ] 代码冻结
- [ ] 最终审查

### 版本准备
- [ ] version.json 更新
- [ ] CHANGELOG 更新
- [ ] 文档更新

### 测试验证
- [ ] 单元测试 ✓
- [ ] 集成测试 ✓
- [ ] E2E 测试 ✓
- [ ] 性能测试 ✓

**准备状态**: ⏳ 进行中 / ✅ 就绪
```

### 6.2 发布执行表

```markdown
## 发布执行记录 - v{VERSION}

**发布日期**: YYYY-MM-DD
**发布人**: @xxx

### 执行步骤
- [ ] 创建 Tag: v{VERSION}
- [ ] CI 构建: [链接]
- [ ] GitHub Release: [链接]
- [ ] 验证完成

### 问题记录
（无 / 列出问题）

**发布状态**: ✅ 成功 / ❌ 失败
```

---

## 7. 发布自动化

### 7.1 GitHub Actions

```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      
      - name: Run tests
        run: go test -race ./...
      
      - name: Build
        run: |
          GOOS=linux GOARCH=amd64 go build -o dist/dep2p-linux-amd64 ./cmd/dep2p
          GOOS=darwin GOARCH=amd64 go build -o dist/dep2p-darwin-amd64 ./cmd/dep2p
          GOOS=windows GOARCH=amd64 go build -o dist/dep2p-windows-amd64.exe ./cmd/dep2p
      
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: dist/*
          generate_release_notes: true
```

---

## 变更历史

| 版本 | 日期 | 作者 | 变更说明 |
|------|------|------|----------|
| v1.0 | 2026-01-11 | DeP2P Team | 初始版本 |
