# core_introspect 模块设计

> **版本**: v1.0.0  
> **更新日期**: 2026-01-18  
> **架构层**: Core Layer  
> **代码位置**: `internal/core/introspect/`

---

## 概述

Introspect 模块提供本地自省 HTTP 服务，用于调试和监控 DeP2P 节点状态。

## 核心功能

1. **诊断端点** - 提供 JSON 格式的节点状态信息
2. **pprof 支持** - Go 运行时性能分析
3. **健康检查** - 标准 `/health` 端点
4. **可配置** - 通过 `config.Diagnostics` 启用/禁用

## 端点列表

| 端点 | 方法 | 说明 |
|------|------|------|
| `/debug/introspect` | GET | 完整诊断报告 |
| `/debug/introspect/node` | GET | 节点信息 |
| `/debug/introspect/connections` | GET | 连接信息 |
| `/debug/introspect/peers` | GET | 节点列表 |
| `/debug/introspect/bandwidth` | GET | 带宽统计 |
| `/debug/introspect/runtime` | GET | 运行时信息 |
| `/debug/pprof/*` | GET | Go pprof 端点 |
| `/health` | GET | 健康检查 |

## 配置

```go
type DiagnosticsConfig struct {
    EnableIntrospect bool   `json:"enable_introspect"`
    IntrospectAddr   string `json:"introspect_addr"`
}
```

默认配置：
- `EnableIntrospect`: `false`（默认禁用）
- `IntrospectAddr`: `"127.0.0.1:6060"`

## 安全考虑

- 默认只监听本地地址 (`127.0.0.1`)
- 生产环境建议禁用或配置访问控制
- pprof 端点可能暴露敏感信息

## 依赖关系

- `pkg/interfaces.Host` - 获取节点信息（可选）
- `pkg/interfaces.ConnManager` - 获取连接信息（可选）
- `BandwidthReporter` - 获取带宽统计（可选）

## 来源

设计来源于 [20260118-additional-feature-absorption.md](../../../_discussions/20260118-additional-feature-absorption.md) Phase 10.2。
