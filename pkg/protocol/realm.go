package protocol

import "fmt"

// ============================================================================
//                         Realm 协议构建器
// ============================================================================
//
// Realm 协议是控制协议，RealmID 嵌入协议路径，需要 Realm 成员资格。
// 格式: /dep2p/realm/<realmID>/<protocol>/<version>

// Realm 协议名称常量
const (
	RealmProtocolAuth        = "auth"
	RealmProtocolSync        = "sync"
	RealmProtocolAnnounce    = "announce"
	RealmProtocolAddressbook = "addressbook"
	RealmProtocolJoin        = "join"
	RealmProtocolRoute       = "route"
)

// RealmBuilder Realm 协议构建器
type RealmBuilder struct {
	realmID string
}

// NewRealmBuilder 创建 Realm 协议构建器
func NewRealmBuilder(realmID string) *RealmBuilder {
	return &RealmBuilder{realmID: realmID}
}

// RealmID 返回构建器关联的 RealmID
func (b *RealmBuilder) RealmID() string {
	return b.realmID
}

// Auth 返回认证协议 ID
// 用于 Realm 成员认证
func (b *RealmBuilder) Auth() ID {
	return ID(fmt.Sprintf("/dep2p/realm/%s/auth/1.0.0", b.realmID))
}

// Sync 返回同步协议 ID
// 用于 Realm 状态同步
func (b *RealmBuilder) Sync() ID {
	return ID(fmt.Sprintf("/dep2p/realm/%s/sync/1.0.0", b.realmID))
}

// Announce 返回公告协议 ID
// 用于 Realm 内公告广播
func (b *RealmBuilder) Announce() ID {
	return ID(fmt.Sprintf("/dep2p/realm/%s/announce/1.0.0", b.realmID))
}

// Addressbook 返回地址簿协议 ID
// 用于 Realm 成员地址管理
func (b *RealmBuilder) Addressbook() ID {
	return ID(fmt.Sprintf("/dep2p/realm/%s/addressbook/1.0.0", b.realmID))
}

// Join 返回加入协议 ID
// 用于加入 Realm
func (b *RealmBuilder) Join() ID {
	return ID(fmt.Sprintf("/dep2p/realm/%s/join/1.0.0", b.realmID))
}

// Route 返回路由协议 ID
// 用于 Realm 内路由发现
func (b *RealmBuilder) Route() ID {
	return ID(fmt.Sprintf("/dep2p/realm/%s/route/1.0.0", b.realmID))
}

// Custom 返回自定义协议 ID
func (b *RealmBuilder) Custom(name, version string) ID {
	return ID(fmt.Sprintf("/dep2p/realm/%s/%s/%s", b.realmID, name, version))
}

// BuildRealmProtocol 便捷函数：构建 Realm 协议 ID
func BuildRealmProtocol(realmID, name, version string) ID {
	return ID(fmt.Sprintf("/dep2p/realm/%s/%s/%s", realmID, name, version))
}

// RealmAuthFormat 返回认证协议的格式字符串（用于兼容旧代码）
// Deprecated: 请使用 NewRealmBuilder(realmID).Auth()
const RealmAuthFormat = "/dep2p/realm/%s/auth/1.0.0"

// RealmSyncFormat 返回同步协议的格式字符串（用于兼容旧代码）
// Deprecated: 请使用 NewRealmBuilder(realmID).Sync()
const RealmSyncFormat = "/dep2p/realm/%s/sync/1.0.0"

// RealmAddressbookFormat 返回地址簿协议的格式字符串（用于兼容旧代码）
// Deprecated: 请使用 NewRealmBuilder(realmID).Addressbook()
const RealmAddressbookFormat = "/dep2p/realm/%s/addressbook/1.0.0"
