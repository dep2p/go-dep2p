package addressbook

import (
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// StoreConfig 存储配置
//
// 从 v1.1.0 开始，AddressBook 统一使用 BadgerDB 持久化存储。
// Engine 参数是必需的。
type StoreConfig struct {
	// Engine BadgerDB 引擎（必需）
	Engine engine.InternalEngine

	// DefaultTTL 默认 TTL
	DefaultTTL time.Duration

	// Prefix 存储前缀（可选，默认为 "addressbook/"）
	Prefix []byte
}

// DefaultStoreConfig 返回默认存储配置
//
// 注意：Engine 必须在使用前设置。
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		DefaultTTL: 24 * time.Hour,
		Prefix:     []byte("addressbook/"),
	}
}

// Validate 验证配置
func (c *StoreConfig) Validate() error {
	if c.Engine == nil {
		return ErrEngineRequired
	}
	return nil
}

// NewStore 创建存储实例
//
// 从 v1.1.0 开始，只支持 BadgerDB 持久化存储。
func NewStore(config StoreConfig) (realmif.AddressBookStore, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	prefix := config.Prefix
	if len(prefix) == 0 {
		prefix = []byte("addressbook/")
	}

	kvStore := kv.New(config.Engine, prefix)
	return NewBadgerStoreWithTTL(kvStore, config.DefaultTTL)
}

// NewBadgerStoreWithEngine 使用引擎创建 BadgerDB 存储
//
// 这是推荐的创建方式，自动处理前缀隔离。
func NewBadgerStoreWithEngine(eng engine.InternalEngine) (*BadgerStore, error) {
	if eng == nil {
		return nil, fmt.Errorf("addressbook: %w", ErrEngineRequired)
	}
	kvStore := kv.New(eng, []byte("addressbook/"))
	return NewBadgerStoreWithTTL(kvStore, 24*time.Hour)
}
