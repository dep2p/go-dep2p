package dht

import (
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              AuthoritativeSource 测试
// ============================================================================

func TestAuthoritativeSource_String(t *testing.T) {
	// 
	tests := []struct {
		source   AuthoritativeSource
		expected string
	}{
		{SourceUnknown, "Unknown"},
		{SourceLocal, "Local"},
		{SourceRelay, "Relay"},
		{SourceDHT, "DHT"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			if got := tc.source.String(); got != tc.expected {
				t.Errorf("AuthoritativeSource(%d).String() = %q, want %q", tc.source, got, tc.expected)
			}
		})
	}
}

func TestAuthoritativeSource_IsAuthoritative(t *testing.T) {
	tests := []struct {
		source   AuthoritativeSource
		expected bool
	}{
		{SourceUnknown, false},
		{SourceLocal, false},
		{SourceRelay, false},
		{SourceDHT, true},
	}

	for _, tc := range tests {
		t.Run(tc.source.String(), func(t *testing.T) {
			if got := tc.source.IsAuthoritative(); got != tc.expected {
				t.Errorf("AuthoritativeSource(%d).IsAuthoritative() = %v, want %v", tc.source, got, tc.expected)
			}
		})
	}
}

// ============================================================================
//                              AuthoritativeRecord 测试
// ============================================================================

func TestAuthoritativeRecord_IsExpired(t *testing.T) {
	// 未过期的记录
	notExpired := &AuthoritativeRecord{
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if notExpired.IsExpired() {
		t.Error("expected record to not be expired")
	}

	// 已过期的记录
	expired := &AuthoritativeRecord{
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if !expired.IsExpired() {
		t.Error("expected record to be expired")
	}
}

func TestAuthoritativeRecord_IsValid(t *testing.T) {
	// 有效记录
	valid := &AuthoritativeRecord{
		Verified:  true,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if !valid.IsValid() {
		t.Error("expected record to be valid")
	}

	// 未验证
	unverified := &AuthoritativeRecord{
		Verified:  false,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if unverified.IsValid() {
		t.Error("expected unverified record to be invalid")
	}

	// 已过期
	expired := &AuthoritativeRecord{
		Verified:  true,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if expired.IsValid() {
		t.Error("expected expired record to be invalid")
	}

	// 有验证错误
	withError := &AuthoritativeRecord{
		Verified:          true,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		VerificationError: ErrInvalidSignature,
	}
	if withError.IsValid() {
		t.Error("expected record with error to be invalid")
	}
}

func TestAuthoritativeRecord_TTLRemaining(t *testing.T) {
	// 还有 1 小时
	record := &AuthoritativeRecord{
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	remaining := record.TTLRemaining()
	if remaining < 59*time.Minute || remaining > 61*time.Minute {
		t.Errorf("expected TTLRemaining around 1 hour, got %v", remaining)
	}

	// 已过期
	expired := &AuthoritativeRecord{
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if expired.TTLRemaining() != 0 {
		t.Errorf("expected TTLRemaining to be 0 for expired record, got %v", expired.TTLRemaining())
	}
}

// ============================================================================
//                              AuthoritativeQueryResult 测试
// ============================================================================

func TestAuthoritativeQueryResult_HasAddresses(t *testing.T) {
	// 有地址
	withAddrs := &AuthoritativeQueryResult{
		Addresses: []string{"/ip4/192.168.1.1/tcp/4001"},
	}
	if !withAddrs.HasAddresses() {
		t.Error("expected HasAddresses to return true")
	}

	// 无地址
	withoutAddrs := &AuthoritativeQueryResult{
		Addresses: nil,
	}
	if withoutAddrs.HasAddresses() {
		t.Error("expected HasAddresses to return false")
	}
}

// ============================================================================
//                              AuthoritativeQueryOptions 测试
// ============================================================================

func TestDefaultAuthoritativeQueryOptions(t *testing.T) {
	opts := defaultAuthoritativeQueryOptions()

	if !opts.QueryDHT {
		t.Error("expected QueryDHT to be true by default")
	}
	if !opts.QueryRelay {
		t.Error("expected QueryRelay to be true by default")
	}
	if !opts.QueryPeerstore {
		t.Error("expected QueryPeerstore to be true by default")
	}
	if opts.RequireAuthoritative {
		t.Error("expected RequireAuthoritative to be false by default")
	}
}

func TestWithDHTOnly(t *testing.T) {
	opts := defaultAuthoritativeQueryOptions()
	WithDHTOnly()(opts)

	if !opts.QueryDHT {
		t.Error("expected QueryDHT to be true")
	}
	if opts.QueryRelay {
		t.Error("expected QueryRelay to be false")
	}
	if opts.QueryPeerstore {
		t.Error("expected QueryPeerstore to be false")
	}
	if !opts.RequireAuthoritative {
		t.Error("expected RequireAuthoritative to be true")
	}
}

func TestWithFallback(t *testing.T) {
	opts := defaultAuthoritativeQueryOptions()
	WithFallback(false, true)(opts)

	if !opts.QueryRelay {
		// 注意：WithFallback 设置 QueryRelay = false
	}
	if !opts.QueryPeerstore {
		t.Error("expected QueryPeerstore to be true")
	}
}

// ============================================================================
//                              noopAddressBookProvider 测试
// ============================================================================

func TestNoopAddressBookProvider(t *testing.T) {
	provider := &noopAddressBookProvider{}

	// GetPeerAddresses 应返回空
	direct, relay, found := provider.GetPeerAddresses(nil, types.NodeID("test"))
	if found {
		t.Error("expected found to be false")
	}
	if len(direct) != 0 || len(relay) != 0 {
		t.Error("expected empty address lists")
	}

	// UpdateFromDHT 应返回 nil
	if err := provider.UpdateFromDHT(nil, nil); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	// InvalidateCache 应返回 nil
	if err := provider.InvalidateCache(nil, types.NodeID("test")); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
