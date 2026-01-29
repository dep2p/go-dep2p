// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Metrics 接口，提供监控指标服务。
package interfaces

import (
	"context"
	"time"
)

// Metrics 定义监控指标服务接口
//
// Metrics 提供 Prometheus/OpenTelemetry 兼容的监控功能。
type Metrics interface {
	// Register 注册指标收集器
	Register(collector MetricsCollector) error

	// Unregister 注销指标收集器
	Unregister(collector MetricsCollector) error

	// GetCounter 获取计数器
	GetCounter(name string) Counter

	// GetGauge 获取仪表
	GetGauge(name string) Gauge

	// GetHistogram 获取直方图
	GetHistogram(name string) Histogram

	// GetSummary 获取摘要
	GetSummary(name string) Summary

	// Snapshot 获取指标快照
	Snapshot() MetricsSnapshot

	// Start 启动指标服务
	Start(ctx context.Context) error

	// Stop 停止指标服务
	Stop(ctx context.Context) error
}

// MetricsCollector 指标收集器接口
type MetricsCollector interface {
	// Collect 收集指标
	Collect(ch chan<- Metric)

	// Describe 描述指标
	Describe(ch chan<- MetricDesc)
}

// Metric 单个指标
type Metric struct {
	// Desc 指标描述
	Desc MetricDesc

	// Value 指标值
	Value float64

	// Labels 标签
	Labels map[string]string

	// Timestamp 时间戳
	Timestamp time.Time
}

// MetricDesc 指标描述
type MetricDesc struct {
	// Name 指标名称
	Name string

	// Help 帮助信息
	Help string

	// Type 指标类型
	Type MetricType
}

// MetricType 指标类型
type MetricType int

const (
	// MetricTypeCounter 计数器
	MetricTypeCounter MetricType = iota
	// MetricTypeGauge 仪表
	MetricTypeGauge
	// MetricTypeHistogram 直方图
	MetricTypeHistogram
	// MetricTypeSummary 摘要
	MetricTypeSummary
)

// Counter 计数器接口
type Counter interface {
	// Inc 增加 1
	Inc()

	// Add 增加指定值
	Add(delta float64)

	// Get 获取当前值
	Get() float64
}

// Gauge 仪表接口
type Gauge interface {
	// Set 设置值
	Set(value float64)

	// Inc 增加 1
	Inc()

	// Dec 减少 1
	Dec()

	// Add 增加指定值
	Add(delta float64)

	// Sub 减少指定值
	Sub(delta float64)

	// Get 获取当前值
	Get() float64
}

// Histogram 直方图接口
type Histogram interface {
	// Observe 观察值
	Observe(value float64)
}

// Summary 摘要接口
type Summary interface {
	// Observe 观察值
	Observe(value float64)
}

// MetricsSnapshot 指标快照
type MetricsSnapshot struct {
	// Timestamp 快照时间
	Timestamp time.Time

	// Connections 连接相关指标
	Connections ConnectionMetrics

	// Bandwidth 带宽相关指标
	Bandwidth BandwidthMetrics

	// Streams 流相关指标
	Streams StreamMetrics
}

// ConnectionMetrics 连接指标
type ConnectionMetrics struct {
	// Total 总连接数
	Total int

	// Inbound 入站连接数
	Inbound int

	// Outbound 出站连接数
	Outbound int
}

// BandwidthMetrics 带宽指标
type BandwidthMetrics struct {
	// TotalIn 总入站流量（字节）
	TotalIn int64

	// TotalOut 总出站流量（字节）
	TotalOut int64

	// RateIn 入站速率（字节/秒）
	RateIn float64

	// RateOut 出站速率（字节/秒）
	RateOut float64
}

// StreamMetrics 流指标
type StreamMetrics struct {
	// Total 总流数
	Total int

	// Inbound 入站流数
	Inbound int

	// Outbound 出站流数
	Outbound int
}
