package noise

import (
	"bytes"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// testKeyFactory 返回测试用的 KeyFactory（直接使用 identity 模块实现）
func testKeyFactory() identityif.KeyFactory {
	return identity.NewKeyFactory()
}

func TestCreateBindingMessage(t *testing.T) {
	noiseKey := make([]byte, NoiseKeySize)
	for i := range noiseKey {
		noiseKey[i] = byte(i)
	}

	msg := CreateBindingMessage(noiseKey)

	// 验证消息长度（SHA256 = 32 bytes）
	if len(msg) != 32 {
		t.Errorf("绑定消息长度应为 32，实际为 %d", len(msg))
	}

	// 验证相同输入产生相同输出
	msg2 := CreateBindingMessage(noiseKey)
	if !bytes.Equal(msg, msg2) {
		t.Error("相同输入应产生相同的绑定消息")
	}

	// 验证不同输入产生不同输出
	noiseKey2 := make([]byte, NoiseKeySize)
	noiseKey2[0] = 0xFF
	msg3 := CreateBindingMessage(noiseKey2)
	if bytes.Equal(msg, msg3) {
		t.Error("不同输入应产生不同的绑定消息")
	}
}

func TestEncodeDecodeIdentityBindingPayload(t *testing.T) {
	// 生成测试 identity
	priv, pub, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	ident := identity.NewIdentityFromKeyPair(priv, pub)

	// 生成测试 Noise 公钥
	noiseKey := make([]byte, NoiseKeySize)
	for i := range noiseKey {
		noiseKey[i] = byte(i)
	}

	// 编码
	payload, err := EncodeIdentityBindingPayload(ident, noiseKey)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	// 验证 payload 不为空
	if len(payload) == 0 {
		t.Fatal("payload 不应为空")
	}

	// 解码并验证（使用 KeyFactory）
	keyFactory := testKeyFactory()
	binding, err := DecodeAndVerifyIdentityBindingPayload(payload, noiseKey, keyFactory)
	if err != nil {
		t.Fatalf("解码验证失败: %v", err)
	}

	// 验证 NodeID 一致
	expectedNodeID := identityif.NodeIDFromPublicKey(ident.PublicKey())
	if !binding.NodeID.Equal(expectedNodeID) {
		t.Errorf("NodeID 不匹配: 期望 %s，实际 %s", expectedNodeID.String(), binding.NodeID.String())
	}

	// 验证公钥一致
	if !binding.PublicKey.Equal(pub) {
		t.Error("公钥不匹配")
	}

	// 验证密钥类型
	if binding.KeyType != types.KeyTypeEd25519 {
		t.Errorf("密钥类型不匹配: 期望 Ed25519，实际 %v", binding.KeyType)
	}
}

func TestDecodeAndVerifyIdentityBindingPayload_InvalidMagic(t *testing.T) {
	payload := []byte("XXXX" + "12345678") // 错误的魔数
	_, err := DecodeAndVerifyIdentityBindingPayload(payload, nil, nil)
	if err != ErrInvalidBindingMagic {
		t.Errorf("期望 ErrInvalidBindingMagic，实际 %v", err)
	}
}

func TestDecodeAndVerifyIdentityBindingPayload_InvalidVersion(t *testing.T) {
	payload := []byte("D2P1" + string([]byte{99})) // 版本 99
	_, err := DecodeAndVerifyIdentityBindingPayload(payload, nil, nil)
	if err == nil {
		t.Error("应返回版本错误")
	}
}

func TestDecodeAndVerifyIdentityBindingPayload_TooShort(t *testing.T) {
	payload := []byte("D2P1")
	_, err := DecodeAndVerifyIdentityBindingPayload(payload, nil, nil)
	if err != ErrInvalidBindingPayload {
		t.Errorf("期望 ErrInvalidBindingPayload，实际 %v", err)
	}
}

func TestDecodeAndVerifyIdentityBindingPayload_WrongNoiseKey(t *testing.T) {
	// 生成测试 identity
	priv, pub, err := identity.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	ident := identity.NewIdentityFromKeyPair(priv, pub)

	// 生成测试 Noise 公钥
	noiseKey := make([]byte, NoiseKeySize)
	for i := range noiseKey {
		noiseKey[i] = byte(i)
	}

	// 编码
	payload, err := EncodeIdentityBindingPayload(ident, noiseKey)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}

	// 使用不同的 Noise 公钥解码（应失败）
	wrongNoiseKey := make([]byte, NoiseKeySize)
	wrongNoiseKey[0] = 0xFF

	keyFactory := testKeyFactory()
	_, err = DecodeAndVerifyIdentityBindingPayload(payload, wrongNoiseKey, keyFactory)
	if err != ErrBindingSignatureInvalid {
		t.Errorf("期望 ErrBindingSignatureInvalid，实际 %v", err)
	}
}

func TestEncodeIdentityBindingPayload_NilIdentity(t *testing.T) {
	noiseKey := make([]byte, NoiseKeySize)
	_, err := EncodeIdentityBindingPayload(nil, noiseKey)
	if err == nil {
		t.Error("nil identity 应返回错误")
	}
}

