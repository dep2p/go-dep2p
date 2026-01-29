package identity

import (
	"testing"
	"time"
)

// ============================================================================
// 签名操作测试
// ============================================================================

// TestSigning_RoundTrip 测试签名-验证循环
func TestSigning_RoundTrip(t *testing.T) {
	priv, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	testData := []byte("test message")

	// 签名
	sig, err := priv.Sign(testData)
	if err != nil {
		t.Errorf("Sign() failed: %v", err)
	}

	// 验证
	valid, err := pub.Verify(testData, sig)
	if err != nil {
		t.Errorf("Verify() failed: %v", err)
	}

	if !valid {
		t.Error("Verify() returned false for valid signature")
	}
}

// TestSigning_InvalidSignature 测试无效签名
func TestSigning_InvalidSignature(t *testing.T) {
	_, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	testData := []byte("test message")
	invalidSig := make([]byte, 64) // 全零签名

	// 验证应该失败
	valid, err := pub.Verify(testData, invalidSig)
	if err != nil {
		t.Errorf("Verify() failed: %v", err)
	}

	if valid {
		t.Error("Verify() returned true for invalid signature")
	}
}

// TestSigning_WrongData 测试错误数据验证失败
func TestSigning_WrongData(t *testing.T) {
	priv, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	data1 := []byte("message 1")
	data2 := []byte("message 2")

	// 签名 data1
	sig, err := priv.Sign(data1)
	if err != nil {
		t.Errorf("Sign() failed: %v", err)
	}

	// 用 data2 验证应该失败
	valid, err := pub.Verify(data2, sig)
	if err != nil {
		t.Errorf("Verify() failed: %v", err)
	}

	if valid {
		t.Error("Verify() returned true for wrong data")
	}
}

// TestSigning_WrongKey 测试错误密钥验证失败
func TestSigning_WrongKey(t *testing.T) {
	priv1, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	_, pub2, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	testData := []byte("test message")

	// 用 priv1 签名
	sig, err := priv1.Sign(testData)
	if err != nil {
		t.Errorf("Sign() failed: %v", err)
	}

	// 用 pub2 验证应该失败
	valid, err := pub2.Verify(testData, sig)
	if err != nil {
		t.Errorf("Verify() failed: %v", err)
	}

	if valid {
		t.Error("Verify() returned true for wrong public key")
	}
}

// ============================================================================
// 基准测试
// ============================================================================

// BenchmarkSign 签名性能（目标 < 1ms）
func BenchmarkSign(b *testing.B) {
	priv, _, _ := GenerateEd25519Key()
	data := []byte("test message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := priv.Sign(data)
		if err != nil {
			b.Fatalf("Sign() failed: %v", err)
		}
	}
}

// BenchmarkVerify 验证性能（目标 < 1ms）
func BenchmarkVerify(b *testing.B) {
	priv, pub, _ := GenerateEd25519Key()
	data := []byte("test message")
	sig, _ := priv.Sign(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pub.Verify(data, sig)
		if err != nil {
			b.Fatalf("Verify() failed: %v", err)
		}
	}
}

// TestPerformance_SignAndVerify 测试性能要求（NFR-ID-002）
func TestPerformance_SignAndVerify(t *testing.T) {
	if testing.Short() {
		return // 性能测试在 short 模式下静默跳过
	}

	priv, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	data := []byte("performance test data")

	// 预热阶段：避免首次调用的冷启动开销影响测试结果
	// 预热也需要验证成功，确保密钥对正常工作
	warmupSig, warmupErr := priv.Sign(data)
	if warmupErr != nil {
		t.Fatalf("Warmup Sign() failed: %v", warmupErr)
	}
	valid, verifyErr := pub.Verify(data, warmupSig)
	if verifyErr != nil || !valid {
		t.Fatalf("Warmup Verify() failed: %v, valid=%v", verifyErr, valid)
	}

	// 性能阈值：2ms（考虑到系统负载波动，比理想的 1ms 稍宽松）
	const threshold = 2 * time.Millisecond

	// 测试签名性能
	start := time.Now()
	sig, err := priv.Sign(data)
	signDuration := time.Since(start)

	if err != nil {
		t.Fatalf("Sign() failed: %v", err)
	}

	if signDuration > threshold {
		t.Errorf("Sign() took %v, want < %v", signDuration, threshold)
	}

	// 测试验证性能
	start = time.Now()
	_, err = pub.Verify(data, sig)
	verifyDuration := time.Since(start)

	if err != nil {
		t.Fatalf("Verify() failed: %v", err)
	}

	if verifyDuration > threshold {
		t.Errorf("Verify() took %v, want < %v", verifyDuration, threshold)
	}

	t.Logf("Performance: Sign=%v, Verify=%v (threshold=%v)", signDuration, verifyDuration, threshold)
}
