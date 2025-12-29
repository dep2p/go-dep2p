// Package dht 提供分布式哈希表实现
package dht

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Seqno 单调递增策略
// ============================================================================

// nextMonotonicSeqno 生成单调递增的 seqno
//
// Layer1 修复：使用时间戳（微秒）作为基础，确保重启后 seqno 仍然单调递增
// 策略：seqno = max(lastSeqno+1, currentTimeMicros)
//
// 优点：
// - 无需持久化（时间自然向前）
// - 重启后 seqno 不会回退（除非时钟回拨）
// - 同一微秒内多次发布通过 lastSeqno+1 保证递增
func (d *DHT) nextMonotonicSeqno() uint64 {
	// 当前时间的微秒数
	nowMicros := uint64(time.Now().UnixMicro())

	for {
		last := atomic.LoadUint64(&d.peerRecordSeqno)
		// 取 max(last+1, nowMicros) 确保单调递增
		next := last + 1
		if nowMicros > next {
			next = nowMicros
		}
		if atomic.CompareAndSwapUint64(&d.peerRecordSeqno, last, next) {
			return next
		}
		// CAS 失败，重试
	}
}

// ============================================================================
//                              PeerRecord 发布
// ============================================================================

// PublishPeerRecord 发布本节点的 PeerRecord 到 DHT
//
// 使用 SignedPeerRecord 格式，确保地址记录不被篡改。
// Key 格式：dep2p/v1/sys/peer/<NodeID>
//
// Layer1 修复：seqno 采用时间戳派生策略，确保重启后仍然单调递增
func (d *DHT) PublishPeerRecord(ctx context.Context, addrs []string) error {
	if atomic.LoadInt32(&d.running) == 0 {
		return ErrDHTClosed
	}

	if len(addrs) == 0 {
		return errors.New("no addresses to publish")
	}

	// Layer1 修复：使用时间戳派生的单调递增 seqno
	// 避免重启后 seqno 回退导致他人拒绝更新
	seqno := d.nextMonotonicSeqno()

	// 获取身份用于签名
	d.identityMu.RLock()
	identity := d.identity
	d.identityMu.RUnlock()

	// Layer1 安全约束：sys/peer 必须使用 SignedPeerRecord，不允许无签名降级
	// 这确保了地址索引的抗投毒能力
	if identity == nil {
		return errors.New("identity required for publishing PeerRecord (no unsigned fallback)")
	}

	// 创建签名的 PeerRecord
	record, err := NewSignedPeerRecord(identity, addrs, seqno, DefaultPeerRecordTTL)
	if err != nil {
		return err
	}
	recordData, err := record.Encode()
	if err != nil {
		return err
	}

	// 生成 key
	key := PeerRecordKeyPrefix + d.localID.String()

	// 发布到 DHT
	if err := d.PutValueWithTTL(ctx, key, recordData, DefaultPeerRecordTTL); err != nil {
		log.Debug("发布 PeerRecord 失败",
			"key", key,
			"err", err)
		return err
	}

	log.Info("已发布 PeerRecord",
		"key", key,
		"addrs", len(addrs),
		"seqno", seqno)

	return nil
}

// LookupPeerRecord 查询指定节点的 PeerRecord
//
// Key 格式：dep2p/v1/sys/peer/<NodeID>
func (d *DHT) LookupPeerRecord(ctx context.Context, nodeID types.NodeID) ([]string, error) {
	if atomic.LoadInt32(&d.running) == 0 {
		return nil, ErrDHTClosed
	}

	key := PeerRecordKeyPrefix + nodeID.String()

	// 从 DHT 获取值
	data, err := d.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}

	// Layer1 安全约束：sys/peer 必须使用 SignedPeerRecord，不允许无签名降级
	// 这确保了地址索引的抗投毒能力

	// 解码为 SignedPeerRecord（不降级到 ProviderRecord）
	signedRecord, err := DecodeSignedPeerRecord(data)
	if err != nil {
		return nil, errors.New("sys/peer requires SignedPeerRecord: " + err.Error())
	}

	// 验证 NodeID 匹配
	if signedRecord.NodeID != nodeID {
		return nil, errors.New("node ID mismatch in peer record")
	}

	// 验证未过期
	if signedRecord.IsExpired() {
		return nil, errors.New("peer record expired")
	}

	// 验证签名
	if err := signedRecord.VerifySelf(); err != nil {
		log.Warn("PeerRecord 签名验证失败",
			"nodeID", nodeID.ShortString(),
			"err", err)
		return nil, errors.New("peer record signature verification failed: " + err.Error())
	}

	return signedRecord.Addrs, nil
}

// ============================================================================
//                              地址更新与重发布
// ============================================================================

// UpdateLocalAddrs 更新本地通告地址
//
// 当节点地址变化时（如 NAT 发现公网地址），调用此方法更新 DHT 的本地地址。
// 实现 discoveryif.AddressUpdater 接口。
func (d *DHT) UpdateLocalAddrs(addrs []string) {
	d.storeMu.Lock()
	d.localAddrs = addrs
	d.storeMu.Unlock()

	// T5 修复：同步更新网络适配器的本地地址
	d.networkMu.RLock()
	network := d.network
	d.networkMu.RUnlock()

	if network != nil {
		network.UpdateLocalAddrs(addrs)
	}

	log.Debug("更新本地地址", "addrs", addrs)

	// 如果正在运行，重新发布 Provider 记录
	if atomic.LoadInt32(&d.running) == 1 {
		go d.republishLocalAddrs()
	}
}

// republishLocalAddrs 重新发布本地地址到 DHT
func (d *DHT) republishLocalAddrs() {
	if d.ctx == nil {
		return
	}

	d.storeMu.RLock()
	addrs := make([]string, len(d.localAddrs))
	copy(addrs, d.localAddrs)
	d.storeMu.RUnlock()

	if len(addrs) == 0 {
		return
	}

	// 使用新的 PublishPeerRecord 方法（支持签名）
	ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
	defer cancel()

	if err := d.PublishPeerRecord(ctx, addrs); err != nil {
		log.Debug("发布地址记录失败", "err", err)
	}
}

