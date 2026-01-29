# 版本治理 (Versioning)

> 兼容性策略、废弃流程

---

## 目录结构

```
versioning/
├── README.md              # 本文件
├── compatibility.md       # 兼容性策略
└── deprecation.md         # 废弃策略
```

---

## 概述

版本治理定义 DeP2P 的兼容性承诺和变更管理策略，确保用户可以平滑升级。

---

## 版本状态

| 版本范围 | 状态 | 兼容性承诺 |
|----------|------|-----------|
| v0.x.x | 开发中 | API 可能变化 |
| v1.x.x | 稳定 | 次版本内向后兼容 |
| v2.x.x+ | 未来 | 次版本内向后兼容 |

---

## 快速链接

| 文档 | 说明 |
|------|------|
| [compatibility.md](compatibility.md) | 兼容性策略 |
| [deprecation.md](deprecation.md) | 废弃策略 |

---

**最后更新**：2026-01-11
