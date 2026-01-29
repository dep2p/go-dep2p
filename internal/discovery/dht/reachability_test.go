package dht

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              DefaultReachabilityChecker 测试
// ============================================================================

func TestDefaultReachabilityChecker_CheckReachability(t *testing.T) {
	tests := []struct {
		name                string
		natType             types.NATType
		expectedReach       types.Reachability
		expectedNAT         types.NATType
	}{
		{
			name:          "None NAT (Public)",
			natType:       types.NATTypeNone,
			expectedReach: types.ReachabilityPublic,
			expectedNAT:   types.NATTypeNone,
		},
		{
			name:          "FullCone NAT (Public)",
			natType:       types.NATTypeFullCone,
			expectedReach: types.ReachabilityPublic,
			expectedNAT:   types.NATTypeFullCone,
		},
		{
			name:          "RestrictedCone NAT (Unknown)",
			natType:       types.NATTypeRestrictedCone,
			expectedReach: types.ReachabilityUnknown,
			expectedNAT:   types.NATTypeRestrictedCone,
		},
		{
			name:          "PortRestricted NAT (Unknown)",
			natType:       types.NATTypePortRestricted,
			expectedReach: types.ReachabilityUnknown,
			expectedNAT:   types.NATTypePortRestricted,
		},
		{
			name:          "Symmetric NAT (Private)",
			natType:       types.NATTypeSymmetric,
			expectedReach: types.ReachabilityPrivate,
			expectedNAT:   types.NATTypeSymmetric,
		},
		{
			name:          "Unknown NAT",
			natType:       types.NATTypeUnknown,
			expectedReach: types.ReachabilityUnknown,
			expectedNAT:   types.NATTypeUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checker := NewDefaultReachabilityChecker(func() types.NATType {
				return tc.natType
			})

			reach, nat := checker.CheckReachability(context.Background())
			if reach != tc.expectedReach {
				t.Errorf("CheckReachability() reachability = %v, want %v", reach, tc.expectedReach)
			}
			if nat != tc.expectedNAT {
				t.Errorf("CheckReachability() natType = %v, want %v", nat, tc.expectedNAT)
			}
		})
	}
}

func TestDefaultReachabilityChecker_NilProvider(t *testing.T) {
	checker := NewDefaultReachabilityChecker(nil)

	reach, nat := checker.CheckReachability(context.Background())
	if reach != types.ReachabilityUnknown {
		t.Errorf("expected ReachabilityUnknown, got %v", reach)
	}
	if nat != types.NATTypeUnknown {
		t.Errorf("expected NATTypeUnknown, got %v", nat)
	}
}

func TestDefaultReachabilityChecker_VerifyAddresses_Public(t *testing.T) {
	checker := NewDefaultReachabilityChecker(func() types.NATType {
		return types.NATTypeNone // Public
	})

	addrs := []string{
		"/ip4/1.2.3.4/tcp/4001",
		"/ip4/5.6.7.8/tcp/4001/p2p-circuit/p2p/relay-id",
	}

	verified, unverified := checker.VerifyAddresses(context.Background(), addrs)

	// Public 节点，所有地址都应该被验证
	if len(verified) != 2 {
		t.Errorf("expected 2 verified addresses, got %d", len(verified))
	}
	if len(unverified) != 0 {
		t.Errorf("expected 0 unverified addresses, got %d", len(unverified))
	}
}

func TestDefaultReachabilityChecker_VerifyAddresses_Private(t *testing.T) {
	checker := NewDefaultReachabilityChecker(func() types.NATType {
		return types.NATTypeSymmetric // Private
	})

	addrs := []string{
		"/ip4/1.2.3.4/tcp/4001",
		"/ip4/5.6.7.8/tcp/4001/p2p-circuit/p2p/relay-id",
	}

	verified, unverified := checker.VerifyAddresses(context.Background(), addrs)

	// Private 节点，只有 Relay 地址被验证
	if len(verified) != 1 {
		t.Errorf("expected 1 verified address (relay), got %d", len(verified))
	}
	if len(unverified) != 1 {
		t.Errorf("expected 1 unverified address (direct), got %d", len(unverified))
	}
}

func TestDefaultReachabilityChecker_IsDirectlyReachable(t *testing.T) {
	// Public
	publicChecker := NewDefaultReachabilityChecker(func() types.NATType {
		return types.NATTypeNone
	})
	if !publicChecker.IsDirectlyReachable(context.Background()) {
		t.Error("expected IsDirectlyReachable to return true for public node")
	}

	// Private
	privateChecker := NewDefaultReachabilityChecker(func() types.NATType {
		return types.NATTypeSymmetric
	})
	if privateChecker.IsDirectlyReachable(context.Background()) {
		t.Error("expected IsDirectlyReachable to return false for private node")
	}
}

// ============================================================================
//                              DynamicTTLCalculator 测试
// ============================================================================

func TestDynamicTTLCalculator_CalculateTTL(t *testing.T) {
	calculator := NewDynamicTTLCalculator()

	tests := []struct {
		name        string
		natType     types.NATType
		frequency   float64
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{
			name:        "Public (None)",
			natType:     types.NATTypeNone,
			frequency:   0,
			minExpected: MaxPeerRecordTTL - time.Hour,
			maxExpected: MaxPeerRecordTTL + time.Hour,
		},
		{
			name:        "FullCone",
			natType:     types.NATTypeFullCone,
			frequency:   0,
			minExpected: DefaultPeerRecordTTL,
			maxExpected: MaxPeerRecordTTL,
		},
		{
			name:        "RestrictedCone",
			natType:     types.NATTypeRestrictedCone,
			frequency:   0,
			minExpected: DefaultPeerRecordTTL / 2,
			maxExpected: DefaultPeerRecordTTL * 2,
		},
		{
			name:        "Symmetric",
			natType:     types.NATTypeSymmetric,
			frequency:   0,
			minExpected: MinPeerRecordTTL,
			maxExpected: DefaultPeerRecordTTL,
		},
		{
			name:        "Unknown",
			natType:     types.NATTypeUnknown,
			frequency:   0,
			minExpected: MinPeerRecordTTL,
			maxExpected: DefaultPeerRecordTTL,
		},
		{
			name:        "Symmetric with high frequency",
			natType:     types.NATTypeSymmetric,
			frequency:   5.0, // 每小时5次变化
			minExpected: MinPeerRecordTTL,
			maxExpected: DefaultPeerRecordTTL / 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ttl := calculator.CalculateTTL(tc.natType, tc.frequency)
			if ttl < tc.minExpected || ttl > tc.maxExpected {
				t.Errorf("CalculateTTL(%v, %f) = %v, expected between %v and %v",
					tc.natType, tc.frequency, ttl, tc.minExpected, tc.maxExpected)
			}
		})
	}
}

func TestDynamicTTLCalculator_Bounds(t *testing.T) {
	calculator := NewDynamicTTLCalculator()

	// 确保 TTL 不会低于 MinPeerRecordTTL
	ttl := calculator.CalculateTTL(types.NATTypeSymmetric, 100) // 极高频率
	if ttl < MinPeerRecordTTL {
		t.Errorf("TTL %v is below minimum %v", ttl, MinPeerRecordTTL)
	}

	// 确保 TTL 不会高于 MaxPeerRecordTTL
	ttl = calculator.CalculateTTL(types.NATTypeNone, 0)
	if ttl > MaxPeerRecordTTL {
		t.Errorf("TTL %v is above maximum %v", ttl, MaxPeerRecordTTL)
	}
}

// ============================================================================
//                              PublishDecision 测试
// ============================================================================

func TestPublishDecision_Fields(t *testing.T) {
	decision := &PublishDecision{
		ShouldPublish: true,
		DirectAddrs:   []string{"/ip4/1.2.3.4/tcp/4001"},
		RelayAddrs:    []string{"/ip4/5.6.7.8/tcp/4001/p2p-circuit/p2p/relay-id"},
		TTL:           1 * time.Hour,
		Reason:        "test",
		Warnings:      []string{"warning1"},
	}

	if !decision.ShouldPublish {
		t.Error("expected ShouldPublish to be true")
	}
	if len(decision.DirectAddrs) != 1 {
		t.Errorf("expected 1 direct addr, got %d", len(decision.DirectAddrs))
	}
	if len(decision.RelayAddrs) != 1 {
		t.Errorf("expected 1 relay addr, got %d", len(decision.RelayAddrs))
	}
	if decision.TTL != 1*time.Hour {
		t.Errorf("expected TTL 1h, got %v", decision.TTL)
	}
	if decision.Reason != "test" {
		t.Errorf("expected reason 'test', got %q", decision.Reason)
	}
	if len(decision.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(decision.Warnings))
	}
}
