// Package interfaces 定义 realm 模块内部接口
package interfaces

import (
	"context"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Realm Realm 接口
//
// v2.0 统一 Relay 架构：Relay 功能已移至节点级别
// Realm 不再直接管理 Relay 连接
type Realm interface {
	ID() string
	Name() string
	Join(ctx context.Context) error
	Leave(ctx context.Context) error
	Members() []string
	IsMember(peerID string) bool
	Connect(ctx context.Context, target string) (pkgif.Connection, error)
	ConnectWithHint(ctx context.Context, target string, hints []string) (pkgif.Connection, error)

	Close() error
}

// Manager 管理器接口
type Manager interface {
	Create(ctx context.Context, id, name string, psk []byte) (Realm, error)
	Get(id string) (Realm, bool)
	List() []Realm
	Close() error
}
