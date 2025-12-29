package app

import (
	"github.com/dep2p/go-dep2p/internal/config"
)

// BootstrapOption Bootstrap 配置选项
type BootstrapOption func(*Bootstrap)

// WithConfig 设置配置
func WithConfig(cfg *config.Config) BootstrapOption {
	return func(b *Bootstrap) {
		b.config = cfg
	}
}

// BuildOptions 构建选项
type BuildOptions struct {
	// StartTimeout 启动超时
	StartTimeout int

	// StopTimeout 停止超时
	StopTimeout int

	// EnableMetrics 启用指标
	EnableMetrics bool

	// EnablePprof 启用 pprof
	EnablePprof bool
}

// DefaultBuildOptions 默认构建选项
func DefaultBuildOptions() BuildOptions {
	return BuildOptions{
		StartTimeout:  30,
		StopTimeout:   30,
		EnableMetrics: false,
		EnablePprof:   false,
	}
}

