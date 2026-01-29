package identify

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
//                     Push 测试
// ============================================================================

func TestService_Push(t *testing.T) {
	service := NewService(nil, nil)

	err := service.Push(context.Background(), "peer-123")
	assert.ErrorIs(t, err, ErrPushNotImplemented)
}

func TestService_Push_MultipleCallsReturnSameError(t *testing.T) {
	service := NewService(nil, nil)

	err1 := service.Push(context.Background(), "peer-1")
	err2 := service.Push(context.Background(), "peer-2")
	err3 := service.Push(context.Background(), "peer-3")

	assert.ErrorIs(t, err1, ErrPushNotImplemented)
	assert.ErrorIs(t, err2, ErrPushNotImplemented)
	assert.ErrorIs(t, err3, ErrPushNotImplemented)
}

// ============================================================================
//                     getProtocols 测试
// ============================================================================

func TestService_GetProtocols_NilRegistry(t *testing.T) {
	service := NewService(nil, nil)
	protocols := service.getProtocols()
	assert.Empty(t, protocols)
}

// ============================================================================
//                     Error 测试
// ============================================================================

func TestErrPushNotImplemented(t *testing.T) {
	assert.NotNil(t, ErrPushNotImplemented)
	assert.Contains(t, ErrPushNotImplemented.Error(), "not implemented")
}

func TestErrPushNotImplemented_IsError(t *testing.T) {
	err := ErrPushNotImplemented
	assert.True(t, errors.Is(err, ErrPushNotImplemented))
}

// ============================================================================
//                     IdentifyInfo 结构测试
// ============================================================================

func TestIdentifyInfo_FullStructure(t *testing.T) {
	info := &IdentifyInfo{
		PeerID:          "peer-123",
		PublicKey:       "base64key==",
		ListenAddrs:     []string{"/ip4/127.0.0.1/tcp/4001", "/ip6/::1/tcp/4001"},
		ObservedAddr:    "/ip4/192.168.1.1/tcp/5001",
		Protocols:       []string{"/test/1.0.0", "/test/2.0.0"},
		AgentVersion:    "go-dep2p/1.0.0",
		ProtocolVersion: "dep2p/1.0.0",
	}

	assert.Equal(t, "peer-123", info.PeerID)
	assert.Equal(t, "base64key==", info.PublicKey)
	assert.Len(t, info.ListenAddrs, 2)
	assert.Equal(t, "/ip4/192.168.1.1/tcp/5001", info.ObservedAddr)
	assert.Len(t, info.Protocols, 2)
	assert.Equal(t, "go-dep2p/1.0.0", info.AgentVersion)
	assert.Equal(t, "dep2p/1.0.0", info.ProtocolVersion)
}

func TestIdentifyInfo_AllFieldsEmpty(t *testing.T) {
	info := &IdentifyInfo{}

	assert.Empty(t, info.PeerID)
	assert.Empty(t, info.PublicKey)
	assert.Empty(t, info.ListenAddrs)
	assert.Empty(t, info.ObservedAddr)
	assert.Empty(t, info.Protocols)
	assert.Empty(t, info.AgentVersion)
	assert.Empty(t, info.ProtocolVersion)
}

// ============================================================================
//                     Constants 测试
// ============================================================================

func TestProtocolID_Values(t *testing.T) {
	assert.Equal(t, "/dep2p/sys/identify/1.0.0", ProtocolID)
	assert.Equal(t, "/dep2p/sys/identify/push/1.0.0", ProtocolIDPush)
}

func TestProtocolID_NotEmpty(t *testing.T) {
	assert.NotEmpty(t, ProtocolID)
	assert.NotEmpty(t, ProtocolIDPush)
}

// ============================================================================
//                     NewService 测试
// ============================================================================

func TestNewService_NilParams(t *testing.T) {
	service := NewService(nil, nil)
	assert.NotNil(t, service)
	assert.Nil(t, service.host)
	assert.Nil(t, service.registry)
}

func TestNewService_ReturnsNonNil(t *testing.T) {
	service := NewService(nil, nil)
	assert.NotNil(t, service)
}
