# 模板库 (Templates)

> 设计文档模板，确保文档格式一致

---

## 目录结构

```
templates/
├── README.md              # 本文件
├── adr_template.md        # ADR 模板
├── req_template.md        # 需求模板
├── arch_template.md       # 架构文档模板
├── tst_template.md        # 测试用例模板
├── bench_template.md      # 基准测试模板
├── proposal_template.md   # 提案模板
└── release_template.md    # 发布说明模板
```

---

## 模板索引

| 模板 | 用途 | 使用场景 |
|------|------|----------|
| [adr_template.md](adr_template.md) | ADR 模板 | 记录架构决策 |
| [req_template.md](req_template.md) | 需求模板 | 定义功能需求 |
| [arch_template.md](arch_template.md) | 架构模板 | 模块架构设计 |
| [tst_template.md](tst_template.md) | 测试模板 | 测试用例设计 |
| [bench_template.md](bench_template.md) | 基准模板 | 性能基准测试 |
| [proposal_template.md](proposal_template.md) | 提案模板 | 改进提案 |
| [release_template.md](release_template.md) | 发布模板 | 版本发布说明 |

---

## 使用说明

1. 复制对应模板
2. 重命名为目标文件名
3. 填写所有必填字段（标记 `[必填]`）
4. 可选字段按需填写

---

## 模板规范

### 元信息

每个文档开头应包含元信息：

```markdown
# 文档标题

> 模板版本: v1.0

---

## 元信息

| 字段 | 值 |
|------|-----|
| **ID** | XXX-NNNN |
| **状态** | draft / approved / ... |
| **创建日期** | YYYY-MM-DD |
| **更新日期** | YYYY-MM-DD |
```

### 变更历史

文档末尾应包含变更历史：

```markdown
## 变更历史

| 版本 | 日期 | 作者 | 变更说明 |
|------|------|------|----------|
| v1.0 | YYYY-MM-DD | @xxx | 初始版本 |
```

---

**最后更新**：2026-01-11
