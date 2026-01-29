# PathHealth 模块设计

> Core Layer - 路径健康管理

## 概述

pathhealth 模块负责监控和管理到各节点的路径健康状态。

## 核心功能

1. **延迟监控** - 测量到节点的延迟
2. **健康评分** - 计算路径健康分数
3. **路径选择** - 辅助连接路径选择
4. **异常检测** - 检测路径异常

## 代码位置

```
internal/core/pathhealth/
├── doc.go          # 包文档
├── manager.go      # 健康管理器
├── manager_test.go
├── path.go         # 路径定义
├── config.go       # 配置定义
└── module.go       # Fx 模块定义
```

## 公共接口

```
pkg/interfaces/pathhealth.go
```

## 依赖

- Peerstore 模块
- Metrics 模块
