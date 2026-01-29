# 发布流程 (Release)

> 版本策略、发布流程、发布产物

---

## 目录结构

```
release/
├── README.md              # 本文件
├── versioning/            # 版本策略
│   ├── README.md
│   └── policy.md          # 版本策略文档
├── process/               # 发布流程
│   ├── README.md
│   ├── checklist.md       # 发布检查清单
│   └── rollback.md        # 回滚策略
└── publishing/            # 发布产物
    ├── README.md
    ├── release.config.yml # 发布配置
    ├── templates/         # 发布模板
    └── versions/          # 版本记录
        └── v0.1.0-alpha/
            └── release.md
```

---

## 版本策略

### 语义化版本

遵循 [SemVer 2.0.0](https://semver.org/)：

```
v{MAJOR}.{MINOR}.{PATCH}[-{prerelease}][+{build}]

示例：
- v1.0.0        # 正式版本
- v1.1.0        # 新功能
- v1.1.1        # Bug 修复
- v2.0.0-alpha  # 下一个大版本预览
- v2.0.0-rc.1   # 候选版本
```

### 版本变更规则

| 变更类型 | 版本升级 | 示例 |
|----------|----------|------|
| 破坏性变更 | MAJOR | v1.x.x → v2.0.0 |
| 新功能（兼容） | MINOR | v1.0.x → v1.1.0 |
| Bug 修复 | PATCH | v1.0.0 → v1.0.1 |

---

## 发布流程

### 发布检查清单

1. **准备阶段**
   - [ ] 确认所有功能已合并
   - [ ] 更新 CHANGELOG.md
   - [ ] 更新 version.json

2. **测试阶段**
   - [ ] 所有单元测试通过
   - [ ] 集成测试通过
   - [ ] E2E 测试通过

3. **发布阶段**
   - [ ] 创建 Release Tag
   - [ ] 生成发布说明
   - [ ] 发布到 GitHub

4. **验证阶段**
   - [ ] 验证发布产物
   - [ ] 通知用户

---

## 发布渠道

| 渠道 | 分支 | 说明 |
|------|------|------|
| `stable` | `main` | 稳定版本 |
| `rc` | `release/*` | 候选版本 |
| `alpha` | `develop` | 开发版本 |

---

## 相关文档

- [版本策略](versioning/)
- [发布检查清单](process/checklist.md)
- [回滚策略](process/rollback.md)

---

**最后更新**：2026-01-11
