// Package dht 提供分布式哈希表实现
package dht

import (
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              网络层注入
// ============================================================================

// SetNetwork 设置网络层
//
// 用于在 Fx Invoke 阶段注入网络实现，解决构造期依赖环问题。
func (d *DHT) SetNetwork(network Network) {
	d.networkMu.Lock()
	defer d.networkMu.Unlock()

	d.network = network

	// 如果网络层提供了 LocalID 和 LocalAddrs，更新本地信息
	if network != nil {
		localID := network.LocalID()
		if !localID.IsEmpty() {
			d.localID = localID
			// 重新初始化路由表（使用新的 localID）
			d.routingTable = NewRoutingTable(localID, d.realmID)
		}
		if addrs := network.LocalAddrs(); len(addrs) > 0 {
			d.localAddrs = addrs
		}
	}

	log.Info("DHT 网络层已注入",
		"localID", d.localID.ShortString(),
		"network_nil", network == nil)
}

// GetNetwork 获取网络层（线程安全）
func (d *DHT) GetNetwork() Network {
	d.networkMu.RLock()
	defer d.networkMu.RUnlock()
	return d.network
}

// ============================================================================
//                              身份注入
// ============================================================================

// SetIdentity 设置身份（用于签名 PeerRecord）
//
// T3 修复：接受 IdentityWithPubKey 以支持签名验证
func (d *DHT) SetIdentity(identity IdentityWithPubKey) {
	d.identityMu.Lock()
	defer d.identityMu.Unlock()
	d.identity = identity

	if identity != nil {
		d.localID = identity.ID()
	}

	log.Debug("DHT 身份已设置", "localID", d.localID.ShortString())
}

// SetIdentityBasic 设置基础身份（不支持验证）
//
// 兼容旧 Identity 接口，但无法创建可验证的 PeerRecord
func (d *DHT) SetIdentityBasic(identity Identity) {
	d.identityMu.Lock()
	defer d.identityMu.Unlock()

	if identity == nil {
		d.identity = nil
	} else {
		// 包装为不带公钥的适配器
		d.identity = &identityNoPubKey{identity}
	}

	if identity != nil {
		d.localID = identity.ID()
	}

	log.Debug("DHT 基础身份已设置", "localID", d.localID.ShortString())
}

// identityNoPubKey 不带公钥的身份适配器
type identityNoPubKey struct {
	Identity
}

func (a *identityNoPubKey) PubKeyBytes() []byte {
	return nil // 不支持公钥
}

// GetIdentity 获取身份
func (d *DHT) GetIdentity() IdentityWithPubKey {
	d.identityMu.RLock()
	defer d.identityMu.RUnlock()
	return d.identity
}

// ============================================================================
//                              模式管理
// ============================================================================

// Mode 获取 DHT 模式
func (d *DHT) Mode() discoveryif.DHTMode {
	return d.config.Mode
}

// SetMode 设置 DHT 模式
func (d *DHT) SetMode(mode discoveryif.DHTMode) {
	d.config.Mode = mode
	log.Debug("DHT 模式已设置", "mode", dhtModeString(mode))
}

// ============================================================================
//                              AddressBook 注入
// ============================================================================

// SetAddressBook 设置外部地址簿
//
// 用于将 DHT 发现的 closer_peers/providers 地址写入全局 AddressBook，
// 作为 Peerstore 类底座，支撑 Endpoint.Connect() 的地址来源。
//
// 参考：design/protocols/foundation/addressing.md - AddressBook 作为 Peerstore 类底座
func (d *DHT) SetAddressBook(ab AddressBookWriter) {
	d.addressBookMu.Lock()
	defer d.addressBookMu.Unlock()
	d.addressBook = ab
	log.Debug("DHT AddressBook 已设置", "hasAddressBook", ab != nil)
}

// getAddressBook 获取地址簿（线程安全）
func (d *DHT) getAddressBook() AddressBookWriter {
	d.addressBookMu.RLock()
	defer d.addressBookMu.RUnlock()
	return d.addressBook
}

// writeAddrsToAddressBook 将发现的地址写入 AddressBook
//
// 这是 DHT 作为 Peerstore 地址汇聚点的关键实现。
func (d *DHT) writeAddrsToAddressBook(nodeID types.NodeID, addrs []string) {
	if len(addrs) == 0 {
		return
	}
	ab := d.getAddressBook()
	if ab == nil {
		return
	}
	ab.Add(nodeID, addrs...)
}

