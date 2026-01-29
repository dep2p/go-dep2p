// Package addrmgmt 提供地址管理协议的实现
package addrmgmt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAddressRecord(t *testing.T) {
	addrs := []string{"/ip4/1.1.1.1/tcp/4001", "/ip4/2.2.2.2/tcp/4002"}
	record := NewAddressRecord("peer1", addrs, time.Hour)

	assert.Equal(t, "peer1", record.NodeID)
	assert.Equal(t, uint64(1), record.Sequence)
	assert.Len(t, record.Addresses, 2)
	assert.Equal(t, time.Hour, record.TTL)
}

func TestAddressRecord_IsExpired(t *testing.T) {
	record := NewAddressRecord("peer1", []string{}, 100*time.Millisecond)

	assert.False(t, record.IsExpired())

	time.Sleep(150 * time.Millisecond)
	assert.True(t, record.IsExpired())
}

func TestAddressRecord_IsNewerThan(t *testing.T) {
	record1 := NewAddressRecord("peer1", []string{}, time.Hour)
	record2 := NewAddressRecord("peer1", []string{}, time.Hour)

	// 相同序列号
	assert.False(t, record1.IsNewerThan(record2))

	// record2 更新后
	record2.Sequence = 2
	assert.False(t, record1.IsNewerThan(record2))
	assert.True(t, record2.IsNewerThan(record1))

	// nil 比较
	assert.True(t, record1.IsNewerThan(nil))
}

func TestAddressRecord_UpdateAddresses(t *testing.T) {
	record := NewAddressRecord("peer1", []string{"/ip4/1.1.1.1/tcp/4001"}, time.Hour)
	originalSeq := record.Sequence

	record.UpdateAddresses([]string{"/ip4/2.2.2.2/tcp/4002", "/ip4/3.3.3.3/tcp/4003"})

	assert.Equal(t, originalSeq+1, record.Sequence)
	assert.Len(t, record.Addresses, 2)
}

func TestAddressRecord_Clone(t *testing.T) {
	record := NewAddressRecord("peer1", []string{"/ip4/1.1.1.1/tcp/4001"}, time.Hour)
	record.Signature = []byte("signature")

	clone := record.Clone()

	assert.Equal(t, record.NodeID, clone.NodeID)
	assert.Equal(t, record.Sequence, clone.Sequence)
	assert.Equal(t, record.Addresses, clone.Addresses)
	assert.Equal(t, record.Signature, clone.Signature)

	// 修改原记录不影响克隆
	record.Addresses[0] = "modified"
	assert.NotEqual(t, record.Addresses[0], clone.Addresses[0])
}

func TestNewHandler(t *testing.T) {
	h := NewHandler("local-peer")

	assert.NotNil(t, h)
	assert.Equal(t, "local-peer", h.localID)
	assert.NotNil(t, h.records)
	assert.NotNil(t, h.neighbors)
}

func TestHandler_GetRecord(t *testing.T) {
	h := NewHandler("local-peer")

	// 记录不存在
	record := h.GetRecord("peer1")
	assert.Nil(t, record)

	// 添加记录
	h.records["peer1"] = NewAddressRecord("peer1", []string{"/ip4/1.1.1.1/tcp/4001"}, time.Hour)

	record = h.GetRecord("peer1")
	assert.NotNil(t, record)
	assert.Equal(t, "peer1", record.NodeID)
}

func TestHandler_GetAllRecords(t *testing.T) {
	h := NewHandler("local-peer")

	h.records["peer1"] = NewAddressRecord("peer1", []string{}, time.Hour)
	h.records["peer2"] = NewAddressRecord("peer2", []string{}, time.Hour)

	all := h.GetAllRecords()

	assert.Len(t, all, 2)
	assert.Contains(t, all, "peer1")
	assert.Contains(t, all, "peer2")
}

func TestHandler_RemoveRecord(t *testing.T) {
	h := NewHandler("local-peer")

	h.records["peer1"] = NewAddressRecord("peer1", []string{}, time.Hour)
	h.RemoveRecord("peer1")

	assert.Nil(t, h.GetRecord("peer1"))
}

func TestHandler_CleanExpired(t *testing.T) {
	h := NewHandler("local-peer")

	// 添加正常记录
	h.records["peer1"] = NewAddressRecord("peer1", []string{}, time.Hour)

	// 添加过期记录
	expiredRecord := NewAddressRecord("peer2", []string{}, 0)
	expiredRecord.Timestamp = time.Now().Add(-time.Hour)
	h.records["peer2"] = expiredRecord

	count := h.CleanExpired()

	assert.Equal(t, 1, count)
	assert.NotNil(t, h.GetRecord("peer1"))
	assert.Nil(t, h.GetRecord("peer2"))
}

func TestHandler_EncodeDecodeRefreshNotify(t *testing.T) {
	h := NewHandler("local-peer")

	record := &AddressRecord{
		NodeID:    "peer1",
		RealmID:   "realm1",
		Sequence:  42,
		Timestamp: time.Now(),
		Addresses: []string{"/ip4/1.1.1.1/tcp/4001", "/ip4/2.2.2.2/tcp/4002"},
		TTL:       time.Hour,
		Signature: []byte("test-signature"),
	}

	// 编码
	encoded := h.encodeRefreshNotify(record)
	require.NotEmpty(t, encoded)

	// 跳过消息头（5 bytes: type + length）
	decoded, err := h.decodeRefreshNotify(encoded[5:])
	require.NoError(t, err)

	assert.Equal(t, record.NodeID, decoded.NodeID)
	assert.Equal(t, record.RealmID, decoded.RealmID)
	assert.Equal(t, record.Sequence, decoded.Sequence)
	assert.Len(t, decoded.Addresses, 2)
}

func TestHandler_EncodeDecodeQueryResponse(t *testing.T) {
	h := NewHandler("local-peer")

	record := &AddressRecord{
		NodeID:    "peer1",
		Sequence:  42,
		Addresses: []string{"/ip4/1.1.1.1/tcp/4001"},
	}

	// 编码
	encoded := h.encodeQueryResponse(record)
	require.NotEmpty(t, encoded)

	// 跳过消息头（5 bytes: type + length）
	decoded, err := h.decodeQueryResponse(encoded[5:])
	require.NoError(t, err)

	assert.Equal(t, record.NodeID, decoded.NodeID)
	assert.Equal(t, record.Sequence, decoded.Sequence)
	assert.Len(t, decoded.Addresses, 1)
}

func TestHandler_SetSignatureVerifier(t *testing.T) {
	h := NewHandler("local-peer")

	verifyCount := 0
	h.SetSignatureVerifier(func(nodeID string, data, signature []byte) bool {
		verifyCount++
		return true
	})

	assert.NotNil(t, h.verifySignature)
}
