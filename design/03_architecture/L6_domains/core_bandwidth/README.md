# Bandwidth 模块设计

> Core Layer - 带宽统计

## 概述

bandwidth 模块提供带宽统计和计量功能，用于监控网络流量和实施流量控制。

## 核心功能

1. **流量计数** - 统计读写字节数
2. **速率计算** - 计算带宽使用率
3. **周期统计** - 按时间周期汇总流量

## 代码位置

```
internal/core/bandwidth/
├── counter.go       # 流量计数器
├── counter_test.go  # 计数器测试
├── doc.go          # 包文档
├── meter.go        # 速率计量器
└── module.go       # Fx 模块定义
```

## 公共接口

```
pkg/interfaces/bandwidth.go
```

## 依赖

- 无外部依赖，独立工作
- 被 Metrics、ConnMgr 使用
