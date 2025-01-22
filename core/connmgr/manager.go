// connmgr 包为 dep2p 提供连接跟踪和管理接口。
//
// 本包导出的 ConnManager 接口允许 dep2p 对打开的连接总数强制执行上限。
// 为了避免服务中断,连接可以使用元数据进行标记,并可选择性地进行"保护",以确保不会随意切断重要连接。
package connmgr

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
)

// SupportsDecay 评估提供的 ConnManager 是否支持衰减,如果支持,则返回 Decayer 对象。
// 更多信息请参考 Decayer 的 godoc 文档。
func SupportsDecay(mgr ConnManager) (Decayer, bool) {
	d, ok := mgr.(Decayer)
	return d, ok
}

// ConnManager 跟踪与对等点的连接,并允许使用者为每个对等点关联元数据。
//
// 它允许基于实现定义的启发式方法修剪连接。
// ConnManager 允许 dep2p 对打开的连接总数强制执行上限。
//
// 支持衰减标签的 ConnManager 实现了 Decayer 接口。
// 如果支持,请使用 SupportsDecay 函数安全地将实例转换为 Decayer。
type ConnManager interface {
	// TagPeer 使用字符串标记对等点,并为该标记关联一个权重。
	TagPeer(peer.ID, string, int)

	// UntagPeer 从对等点移除标记的值。
	UntagPeer(p peer.ID, tag string)

	// UpsertTag 更新现有标记或插入新标记。
	//
	// 连接管理器调用 upsert 函数,提供标记的当前值(如果不存在则为零)。
	// 返回值用作标记的新值。
	UpsertTag(p peer.ID, tag string, upsert func(int) int)

	// GetTagInfo 返回与对等点关联的元数据,如果没有为该对等点记录元数据则返回 nil。
	GetTagInfo(p peer.ID) *TagInfo

	// TrimOpenConns 基于实现定义的启发式方法终止打开的连接。
	TrimOpenConns(ctx context.Context)

	// Notifee 返回一个可以回调的实现,用于通知已打开和关闭的连接。
	Notifee() network.Notifiee

	// Protect 保护对等点不被修剪其连接。
	//
	// 标记允许系统的不同部分管理保护而不互相干扰。
	//
	// 使用相同标记多次调用 Protect() 是幂等的。
	// 它们不进行引用计数,所以在使用相同标记多次调用 Protect() 后,
	// 使用相同标记的单个 Unprotect() 调用将撤销保护。
	Protect(id peer.ID, tag string)

	// Unprotect 移除可能已在指定标记下对对等点设置的保护。
	//
	// 返回值表示在此调用之后,对等点是否通过其他标记继续受到保护。
	// 更多信息请参见 Protect() 的说明。
	Unprotect(id peer.ID, tag string) (protected bool)

	// IsProtected 如果对等点受某个标记保护则返回 true；
	// 如果标记为空字符串,则当对等点受任何标记保护时返回 true
	IsProtected(id peer.ID, tag string) (protected bool)

	// CheckLimit 如果连接管理器的内部连接限制超过提供的系统限制,则返回错误。
	CheckLimit(l GetConnLimiter) error

	// Close 关闭连接管理器并停止后台进程。
	Close() error
}

// TagInfo 存储与对等点关联的元数据。
type TagInfo struct {
	// FirstSeen 记录第一次看到此对等点的时间戳
	FirstSeen time.Time

	// Value 表示此对等点的总体评分值,由所有标记的权重总和计算得出
	Value int

	// Tags 将标记 ID 映射到数值。
	// 每个标记代表一个特定的属性或行为,其对应的数值表示该属性的权重或重要性。
	// 例如 "app=bitswap:5" 表示此对等点在 bitswap 协议中的权重为 5。
	Tags map[string]int

	// Conns 将连接 ID(如远程多地址)映射到其创建时间。
	// 用于跟踪与此对等点的所有活动连接及其建立时间。
	// 连接 ID 通常是远程多地址的字符串表示。
	Conns map[string]time.Time
}

// GetConnLimiter 提供对组件总连接限制的访问。
type GetConnLimiter interface {
	// GetConnLimit 返回实现组件的总连接限制。
	GetConnLimit() int
}
