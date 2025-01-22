package event

import (
	"github.com/dep2p/go-dep2p/core/record"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// AddrAction 表示对主机监听地址执行的操作。
// 它用于在 EvtLocalAddressesUpdated 事件中为地址变更提供上下文。
type AddrAction int

const (
	// Unknown 表示事件生产者无法确定地址处于当前状态的原因。
	Unknown AddrAction = iota

	// Added 表示该地址是新的,在事件之前不存在。
	Added

	// Maintained 表示该地址在当前和之前的状态之间没有改变。
	Maintained

	// Removed 表示该地址已从主机中移除。
	Removed
)

// UpdatedAddress 用于在 EvtLocalAddressesUpdated 事件中传递地址变更信息。
type UpdatedAddress struct {
	// Address 包含被更新的地址。
	Address ma.Multiaddr

	// Action 表示在事件期间对地址采取的操作。如果事件生产者无法生成差异,则可能为 Unknown。
	Action AddrAction
}

// EvtLocalAddressesUpdated 应在本地主机的监听地址集发生变化时发出。
// 这可能由多种原因引起。例如,我们可能打开了新的中继连接、通过 UPnP 建立了新的 NAT 映射,或被其他对等节点告知了我们的观察地址。
//
// EvtLocalAddressesUpdated 包含当前监听地址的快照,也可能包含当前状态和之前状态之间的差异。
// 如果事件生产者能够创建差异,则 Diffs 字段将为 true,事件消费者可以检查每个 UpdatedAddress 的 Action 字段以查看每个地址是如何被修改的。
//
// 例如,Action 将告诉你 Current 列表中的地址是被事件生产者 Added,还是未经更改而被 Maintained。
// 从主机中移除的地址将具有 Removed 的 AddrAction,并将出现在 Removed 列表中。
//
// 如果事件生产者无法生成差异,则 Diffs 字段将为 false,Removed 列表将始终为空,并且 Current 列表中每个 UpdatedAddress 的 Action 将为 Unknown。
//
// 除了上述内容外,EvtLocalAddressesUpdated 还包含当前监听地址集的更新后的 peer.PeerRecord,它被包装在 record.Envelope 中并由主机的私钥签名。
// 该记录可以以安全和认证的方式与其他对等节点共享,以告知它们我们认为可拨号的地址。
type EvtLocalAddressesUpdated struct {

	// Diffs 表示此事件是否包含主机先前地址集的差异。
	Diffs bool

	// Current 包含主机的所有当前监听地址。
	// 如果 Diffs == true,则每个 UpdatedAddress 的 Action 字段将告诉你一个地址是被 Added,还是从先前状态 Maintained。
	Current []UpdatedAddress

	// Removed 包含从主机中移除的地址。
	// 此字段仅在 Diffs == true 时设置。
	Removed []UpdatedAddress

	// SignedPeerRecord 包含我们自己更新后的 peer.PeerRecord,列出了 Current 中枚举的地址。
	// 它被包装在 record.Envelope 中并由主机的私钥签名。
	SignedPeerRecord *record.Envelope
}
