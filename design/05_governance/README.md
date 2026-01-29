# 治理 (Governance)

> 项目治理：提案流程、版本策略、贡献者指南

---

## 目录结构

```
05_governance/
├── README.md              # 本文件
├── proposals/             # 改进提案
│   ├── README.md
│   └── PROP-template.md
├── versioning/            # 版本治理
│   ├── README.md
│   ├── compatibility.md   # 兼容性策略
│   └── deprecation.md     # 废弃策略
└── community/             # 社区治理
    ├── README.md
    ├── contributing.md    # 贡献指南
    └── code_of_conduct.md # 行为准则
```

---

## 提案流程

### 提案类型

| 类型 | 前缀 | 说明 |
|------|------|------|
| 功能提案 | PROP-FEAT | 新功能 |
| 架构提案 | PROP-ARCH | 架构变更 |
| 协议提案 | PROP-PROTO | 协议变更 |
| 流程提案 | PROP-PROC | 流程改进 |

### 提案状态

```
Draft → Review → Accepted/Rejected → Implemented
```

---

## 版本治理

### 兼容性承诺

| 版本 | 兼容性承诺 |
|------|-----------|
| v1.x.x | API 稳定，向后兼容 |
| v0.x.x | API 可能变化 |

### 废弃流程

1. 标记废弃（至少保留 2 个 minor 版本）
2. 发布废弃警告
3. 下一个 major 版本移除

---

## 社区治理

### 贡献流程

1. Fork 仓库
2. 创建功能分支
3. 提交 PR
4. 代码评审
5. 合并

### 代码评审

- 至少一位维护者批准
- CI 检查通过
- 无冲突

---

## 快速链接

| 文档 | 说明 |
|------|------|
| [proposals/](proposals/) | 改进提案 |
| [versioning/](versioning/) | 版本策略 |
| [community/](community/) | 社区指南 |

---

**最后更新**：2026-01-11
