// Package config 提供统一的配置管理
package config

import (
	"fmt"
	"path/filepath"
)

// StorageConfig 存储配置
//
// 配置 DeP2P 的数据存储目录。所有组件统一使用 BadgerDB 持久化存储，
// 通过 Key 前缀隔离不同组件的数据。
//
// 数据目录结构：
//
//	${DataDir}/
//	├── dep2p.db/           # BadgerDB 主数据库
//	│   ├── 000001.vlog     # Value Log
//	│   ├── 000001.sst      # SSTable
//	│   └── MANIFEST        # 数据库元信息
//	└── logs/               # 日志目录（可选）
//	    └── dep2p.log
type StorageConfig struct {
	// DataDir 数据目录路径
	// 存放 BadgerDB 数据库和其他持久化数据
	// 默认值: "./data"
	DataDir string `json:"data_dir"`
}

// DefaultStorageConfig 返回默认的存储配置
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		DataDir: "./data",
	}
}

// Validate 验证存储配置的有效性
func (c *StorageConfig) Validate() error {
	if c.DataDir == "" {
		return fmt.Errorf("storage: data_dir cannot be empty")
	}
	return nil
}

// DBPath 返回 BadgerDB 数据库路径
func (c *StorageConfig) DBPath() string {
	return filepath.Join(c.DataDir, "dep2p.db")
}

// LogPath 返回日志目录路径
func (c *StorageConfig) LogPath() string {
	return filepath.Join(c.DataDir, "logs")
}
