# 第一次贡献

> 新手引导：从零开始参与 DeP2P 开发

---

## 概述

本指南帮助您完成第一次 DeP2P 贡献，从找到合适的任务到成功合并 PR。

---

## 1. 寻找任务

### 1.1 Good First Issues

查看标有 `good first issue` 标签的 Issue：

- [Good First Issues](https://github.com/dep2p/dep2p/labels/good%20first%20issue)

这些任务适合新贡献者：
- 范围清晰
- 难度适中
- 有明确的完成标准

### 1.2 其他贡献方式

| 类型 | 说明 |
|------|------|
| 文档改进 | 修复错别字、改进说明 |
| 测试补充 | 增加测试覆盖 |
| 代码清理 | 重构、优化 |
| Bug 修复 | 修复已知问题 |

---

## 2. 开发流程

### 2.1 Fork 和克隆

```bash
# 1. 在 GitHub 上 Fork 仓库

# 2. 克隆您的 Fork
git clone https://github.com/YOUR_USERNAME/dep2p.git
cd dep2p

# 3. 添加上游仓库
git remote add upstream https://github.com/dep2p/dep2p.git
```

### 2.2 创建分支

```bash
# 同步上游
git fetch upstream
git checkout main
git merge upstream/main

# 创建功能分支
git checkout -b fix/issue-123-description
```

**分支命名规范**：
- `fix/issue-123-description` - Bug 修复
- `feat/feature-name` - 新功能
- `docs/doc-name` - 文档更新

### 2.3 开发

```bash
# 编写代码...

# 运行测试
go test ./...

# 运行 lint
golangci-lint run

# 格式化代码
go fmt ./...
```

### 2.4 提交

```bash
# 添加文件
git add .

# 提交（遵循规范）
git commit -m "fix(transport): handle connection timeout correctly

Fixes #123"
```

**提交消息格式**：

```
type(scope): description

[optional body]

[optional footer]
```

### 2.5 推送

```bash
git push origin fix/issue-123-description
```

---

## 3. 创建 Pull Request

### 3.1 在 GitHub 上创建 PR

1. 打开您的 Fork 页面
2. 点击 "Compare & pull request"
3. 填写 PR 模板

### 3.2 PR 模板

```markdown
## 变更说明

简要描述此 PR 的变更内容...

## 关联 Issue

Fixes #123

## 变更类型

- [x] Bug 修复
- [ ] 新功能
- [ ] 文档更新

## 检查清单

- [x] 代码遵循项目风格
- [x] 添加了必要的测试
- [x] 所有测试通过
- [ ] 更新了相关文档
```

---

## 4. 代码评审

### 4.1 响应评审意见

- 认真阅读每条评审意见
- 及时回复和讨论
- 根据反馈修改代码

### 4.2 更新 PR

```bash
# 修改代码后
git add .
git commit -m "address review comments"
git push
```

### 4.3 保持同步

```bash
# 如果 main 分支有更新
git fetch upstream
git rebase upstream/main
git push -f
```

---

## 5. 合并后

### 5.1 清理分支

```bash
# 删除本地分支
git checkout main
git branch -d fix/issue-123-description

# 删除远程分支
git push origin --delete fix/issue-123-description
```

### 5.2 同步 Fork

```bash
git fetch upstream
git checkout main
git merge upstream/main
git push origin main
```

---

## 6. 提示和技巧

### 6.1 常见问题

**问题：CI 检查失败**

```bash
# 在本地运行相同的检查
go test ./...
golangci-lint run
```

**问题：合并冲突**

```bash
git fetch upstream
git rebase upstream/main
# 解决冲突
git add .
git rebase --continue
git push -f
```

### 6.2 最佳实践

- 保持 PR 小而专注
- 一个 PR 只做一件事
- 写清晰的提交消息
- 添加必要的测试
- 响应评审要及时

---

## 7. 获取帮助

如果遇到问题：

- 在 PR 中提问
- 在 Discussions 中讨论
- 查看文档和示例

---

## 恭喜！

完成第一次贡献后，您已经是 DeP2P 贡献者了！

继续探索：
- 挑战更复杂的任务
- 参与设计讨论
- 帮助其他新贡献者

---

**最后更新**：2026-01-11
