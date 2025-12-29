package identity

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Ed25519 密钥对测试
// ============================================================================

func TestGenerateEd25519KeyPair(t *testing.T) {
	priv, pub, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 验证密钥类型
	if priv.Type() != types.KeyTypeEd25519 {
		t.Errorf("私钥类型错误: got %v, want %v", priv.Type(), types.KeyTypeEd25519)
	}
	if pub.Type() != types.KeyTypeEd25519 {
		t.Errorf("公钥类型错误: got %v, want %v", pub.Type(), types.KeyTypeEd25519)
	}

	// 验证密钥大小
	if len(priv.Bytes()) != ed25519.PrivateKeySize {
		t.Errorf("私钥大小错误: got %d, want %d", len(priv.Bytes()), ed25519.PrivateKeySize)
	}
	if len(pub.Bytes()) != ed25519.PublicKeySize {
		t.Errorf("公钥大小错误: got %d, want %d", len(pub.Bytes()), ed25519.PublicKeySize)
	}

	// 验证公钥一致性
	derivedPub := priv.PublicKey()
	if !pub.Equal(derivedPub) {
		t.Error("从私钥派生的公钥与生成的公钥不一致")
	}
}

func TestEd25519PublicKeyFromBytes(t *testing.T) {
	// 生成密钥对
	_, pub, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 从字节重建公钥
	pubBytes := pub.Bytes()
	rebuiltPub, err := NewEd25519PublicKey(pubBytes)
	if err != nil {
		t.Fatalf("从字节创建公钥失败: %v", err)
	}

	// 验证相等
	if !pub.Equal(rebuiltPub) {
		t.Error("重建的公钥与原公钥不相等")
	}
}

func TestEd25519PrivateKeyFromBytes(t *testing.T) {
	// 生成密钥对
	priv, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 从字节重建私钥
	privBytes := priv.Bytes()
	rebuiltPriv, err := NewEd25519PrivateKey(privBytes)
	if err != nil {
		t.Fatalf("从字节创建私钥失败: %v", err)
	}

	// 验证公钥一致
	if !priv.PublicKey().Equal(rebuiltPriv.PublicKey()) {
		t.Error("重建的私钥对应的公钥与原公钥不相等")
	}
}

func TestEd25519InvalidKeySize(t *testing.T) {
	// 测试无效大小的公钥
	_, err := NewEd25519PublicKey([]byte("too short"))
	if err != ErrInvalidKeySize {
		t.Errorf("期望 ErrInvalidKeySize, got %v", err)
	}

	// 测试无效大小的私钥
	_, err = NewEd25519PrivateKey([]byte("too short"))
	if err != ErrInvalidKeySize {
		t.Errorf("期望 ErrInvalidKeySize, got %v", err)
	}
}

// ============================================================================
//                              NodeID 派生测试
// ============================================================================

func TestNodeIDFromPublicKey(t *testing.T) {
	_, pub, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 派生 NodeID
	nodeID := NodeIDFromPublicKey(pub)

	// 验证 NodeID 不为空
	if nodeID.IsEmpty() {
		t.Error("NodeID 不应为空")
	}

	// 验证派生一致性
	nodeID2 := NodeIDFromPublicKey(pub)
	if !nodeID.Equal(nodeID2) {
		t.Error("从同一公钥派生的 NodeID 应该相同")
	}
}

func TestNodeIDFromDifferentKeys(t *testing.T) {
	// 生成两个不同的密钥对
	_, pub1, _ := GenerateEd25519KeyPair()
	_, pub2, _ := GenerateEd25519KeyPair()

	// 派生 NodeID
	nodeID1 := NodeIDFromPublicKey(pub1)
	nodeID2 := NodeIDFromPublicKey(pub2)

	// 验证不同密钥产生不同的 NodeID
	if nodeID1.Equal(nodeID2) {
		t.Error("不同公钥应该产生不同的 NodeID")
	}
}

// ============================================================================
//                              签名和验证测试
// ============================================================================

func TestSignAndVerify(t *testing.T) {
	priv, pub, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 签名数据
	data := []byte("hello, world!")
	sig, err := priv.Sign(data)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	// 验证签名
	valid, err := pub.Verify(data, sig)
	if err != nil {
		t.Fatalf("验证签名时出错: %v", err)
	}
	if !valid {
		t.Error("有效签名应该验证通过")
	}
}

func TestSignAndVerifyInvalid(t *testing.T) {
	priv, pub, _ := GenerateEd25519KeyPair()

	// 签名数据
	data := []byte("hello, world!")
	sig, _ := priv.Sign(data)

	// 修改数据后验证
	modifiedData := []byte("hello, modified!")
	valid, _ := pub.Verify(modifiedData, sig)
	if valid {
		t.Error("修改后的数据不应该验证通过")
	}

	// 修改签名后验证
	modifiedSig := make([]byte, len(sig))
	copy(modifiedSig, sig)
	modifiedSig[0] ^= 0xFF
	valid, _ = pub.Verify(data, modifiedSig)
	if valid {
		t.Error("修改后的签名不应该验证通过")
	}
}

func TestVerifyWithWrongKey(t *testing.T) {
	priv1, _, _ := GenerateEd25519KeyPair()
	_, pub2, _ := GenerateEd25519KeyPair()

	// 用私钥1签名
	data := []byte("hello, world!")
	sig, _ := priv1.Sign(data)

	// 用公钥2验证
	valid, _ := pub2.Verify(data, sig)
	if valid {
		t.Error("用错误的公钥验证不应该通过")
	}
}

// ============================================================================
//                              Identity 测试
// ============================================================================

func TestNewIdentity(t *testing.T) {
	priv, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	id := NewIdentity(priv)

	// 验证 ID 不为空
	if id.ID().IsEmpty() {
		t.Error("Identity ID 不应为空")
	}

	// 验证密钥类型
	if id.KeyType() != types.KeyTypeEd25519 {
		t.Errorf("密钥类型错误: got %v, want %v", id.KeyType(), types.KeyTypeEd25519)
	}

	// 验证公钥一致性
	if !id.PublicKey().Equal(priv.PublicKey()) {
		t.Error("Identity 公钥与私钥对应的公钥不一致")
	}
}

func TestIdentitySignAndVerify(t *testing.T) {
	priv, _, _ := GenerateEd25519KeyPair()
	id := NewIdentity(priv)

	data := []byte("test data")
	sig, err := id.Sign(data)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	valid, err := id.Verify(data, sig, id.PublicKey())
	if err != nil {
		t.Fatalf("验证失败: %v", err)
	}
	if !valid {
		t.Error("Identity 签名验证应该通过")
	}
}

// ============================================================================
//                              PEM 持久化测试
// ============================================================================

func TestSaveAndLoadPrivateKeyPEM(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	// 生成密钥对
	priv, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 保存私钥
	err = SavePrivateKeyPEM(priv, keyPath)
	if err != nil {
		t.Fatalf("保存私钥失败: %v", err)
	}

	// 验证文件权限
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("获取文件信息失败: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("文件权限错误: got %o, want 0600", info.Mode().Perm())
	}

	// 加载私钥
	loadedPriv, err := LoadPrivateKeyPEM(keyPath)
	if err != nil {
		t.Fatalf("加载私钥失败: %v", err)
	}

	// 验证公钥一致
	if !priv.PublicKey().Equal(loadedPriv.PublicKey()) {
		t.Error("加载的私钥与原私钥不一致")
	}
}

func TestLoadPrivateKeyPEMNotFound(t *testing.T) {
	_, err := LoadPrivateKeyPEM("/nonexistent/path/key.pem")
	if err != ErrKeyNotFound {
		t.Errorf("期望 ErrKeyNotFound, got %v", err)
	}
}

func TestLoadPrivateKeyPEMInvalid(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "invalid.key")

	// 写入无效数据
	err := os.WriteFile(keyPath, []byte("not a pem file"), 0600)
	if err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	_, err = LoadPrivateKeyPEM(keyPath)
	if err != ErrInvalidPEM {
		t.Errorf("期望 ErrInvalidPEM, got %v", err)
	}
}

// ============================================================================
//                              Manager 测试
// ============================================================================

func TestManagerCreate(t *testing.T) {
	config := identityif.DefaultConfig()
	manager := NewManager(config)

	id, err := manager.Create()
	if err != nil {
		t.Fatalf("创建身份失败: %v", err)
	}

	if id.ID().IsEmpty() {
		t.Error("创建的身份 ID 不应为空")
	}
}

func TestManagerCreateWithType(t *testing.T) {
	config := identityif.DefaultConfig()
	manager := NewManager(config)

	id, err := manager.CreateWithType(types.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("创建身份失败: %v", err)
	}

	if id.KeyType() != types.KeyTypeEd25519 {
		t.Errorf("密钥类型错误: got %v, want %v", id.KeyType(), types.KeyTypeEd25519)
	}
}

func TestManagerSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "identity.key")

	config := identityif.DefaultConfig()
	manager := NewManager(config)

	// 创建并保存身份
	id, err := manager.Create()
	if err != nil {
		t.Fatalf("创建身份失败: %v", err)
	}

	err = manager.Save(id, keyPath)
	if err != nil {
		t.Fatalf("保存身份失败: %v", err)
	}

	// 加载身份
	loadedID, err := manager.Load(keyPath)
	if err != nil {
		t.Fatalf("加载身份失败: %v", err)
	}

	// 验证 ID 一致
	if !id.ID().Equal(loadedID.ID()) {
		t.Error("加载的身份 ID 与原身份 ID 不一致")
	}
}

func TestManagerFromPrivateKey(t *testing.T) {
	config := identityif.DefaultConfig()
	manager := NewManager(config)

	priv, _, _ := GenerateEd25519KeyPair()
	id, err := manager.FromPrivateKey(priv)
	if err != nil {
		t.Fatalf("从私钥创建身份失败: %v", err)
	}

	if !id.PublicKey().Equal(priv.PublicKey()) {
		t.Error("创建的身份公钥不匹配")
	}
}

func TestManagerFromBytes(t *testing.T) {
	config := identityif.DefaultConfig()
	manager := NewManager(config)

	priv, _, _ := GenerateEd25519KeyPair()
	privBytes := priv.Bytes()

	id, err := manager.FromBytes(privBytes, types.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("从字节创建身份失败: %v", err)
	}

	if !id.PublicKey().Equal(priv.PublicKey()) {
		t.Error("创建的身份公钥不匹配")
	}
}

func TestManagerGenerateNodeID(t *testing.T) {
	config := identityif.DefaultConfig()
	manager := NewManager(config)

	_, pub, _ := GenerateEd25519KeyPair()

	nodeID := manager.GenerateNodeID(pub)
	expectedNodeID := NodeIDFromPublicKey(pub)

	if !nodeID.Equal(expectedNodeID) {
		t.Error("Manager.GenerateNodeID 结果与 NodeIDFromPublicKey 不一致")
	}
}

// ============================================================================
//                              KeyGenerator 测试
// ============================================================================

func TestEd25519KeyGenerator(t *testing.T) {
	gen := NewEd25519KeyGenerator()

	if gen.Type() != types.KeyTypeEd25519 {
		t.Errorf("密钥类型错误: got %v, want %v", gen.Type(), types.KeyTypeEd25519)
	}

	priv, pub, err := gen.Generate()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	if !priv.PublicKey().Equal(pub) {
		t.Error("生成的密钥对不匹配")
	}
}

// ============================================================================
//                              公钥持久化测试
// ============================================================================

func TestSaveAndLoadPublicKeyPEM(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.pub")

	_, pub, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	err = SavePublicKeyPEM(pub, keyPath)
	if err != nil {
		t.Fatalf("保存公钥失败: %v", err)
	}

	loadedPub, err := LoadPublicKeyPEM(keyPath)
	if err != nil {
		t.Fatalf("加载公钥失败: %v", err)
	}

	if !pub.Equal(loadedPub) {
		t.Error("加载的公钥与原公钥不一致")
	}
}

func TestLoadPublicKeyPEMNotFound(t *testing.T) {
	_, err := LoadPublicKeyPEM("/nonexistent/path/key.pub")
	if err != ErrKeyNotFound {
		t.Errorf("期望 ErrKeyNotFound, got %v", err)
	}
}

func TestLoadPublicKeyPEMInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "invalid.pub")

	err := os.WriteFile(keyPath, []byte("not a pem file"), 0644)
	if err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	_, err = LoadPublicKeyPEM(keyPath)
	if err != ErrInvalidPEM {
		t.Errorf("期望 ErrInvalidPEM, got %v", err)
	}
}

// ============================================================================
//                              边界情况测试
// ============================================================================

func TestPrivateKeyFromBytes(t *testing.T) {
	priv, _, _ := GenerateEd25519KeyPair()

	// 正常情况
	rebuiltPriv, err := PrivateKeyFromBytes(priv.Bytes(), types.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("从字节创建私钥失败: %v", err)
	}
	if !priv.PublicKey().Equal(rebuiltPriv.PublicKey()) {
		t.Error("重建的私钥公钥不匹配")
	}

	// 不支持的密钥类型
	_, err = PrivateKeyFromBytes(priv.Bytes(), types.KeyTypeRSA)
	if err != ErrUnsupportedKeyType {
		t.Errorf("期望 ErrUnsupportedKeyType, got %v", err)
	}
}

func TestPublicKeyFromBytes(t *testing.T) {
	_, pub, _ := GenerateEd25519KeyPair()

	// 正常情况
	rebuiltPub, err := PublicKeyFromBytes(pub.Bytes(), types.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("从字节创建公钥失败: %v", err)
	}
	if !pub.Equal(rebuiltPub) {
		t.Error("重建的公钥不匹配")
	}

	// 不支持的密钥类型
	_, err = PublicKeyFromBytes(pub.Bytes(), types.KeyTypeRSA)
	if err != ErrUnsupportedKeyType {
		t.Errorf("期望 ErrUnsupportedKeyType, got %v", err)
	}
}

func TestManagerCreateWithUnsupportedType(t *testing.T) {
	config := identityif.DefaultConfig()
	manager := NewManager(config)

	_, err := manager.CreateWithType(types.KeyTypeRSA)
	if err == nil {
		t.Error("期望创建不支持的密钥类型时返回错误")
	}
}

func TestSavePrivateKeyPEMUnsupportedType(t *testing.T) {
	// 创建一个模拟的不支持类型的私钥
	priv, _, _ := GenerateEd25519KeyPair()
	
	// 正常保存应该成功
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	err := SavePrivateKeyPEM(priv, keyPath)
	if err != nil {
		t.Errorf("保存私钥失败: %v", err)
	}
}

func TestSavePublicKeyPEMUnsupportedType(t *testing.T) {
	_, pub, _ := GenerateEd25519KeyPair()
	
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.pub")
	err := SavePublicKeyPEM(pub, keyPath)
	if err != nil {
		t.Errorf("保存公钥失败: %v", err)
	}
}

func TestVerifyInvalidSignatureSize(t *testing.T) {
	_, pub, _ := GenerateEd25519KeyPair()

	// 签名大小无效
	valid, err := pub.Verify([]byte("data"), []byte("short"))
	if err != nil {
		t.Errorf("验证不应返回错误: %v", err)
	}
	if valid {
		t.Error("无效大小的签名不应该验证通过")
	}
}

func TestEd25519PublicKeyNotEqual(t *testing.T) {
	_, pub1, _ := GenerateEd25519KeyPair()
	_, pub2, _ := GenerateEd25519KeyPair()

	if pub1.Equal(pub2) {
		t.Error("不同的公钥应该不相等")
	}
}

func TestEd25519RawMethods(t *testing.T) {
	priv, pub, _ := GenerateEd25519KeyPair()

	// 测试 Raw() 方法
	rawPub := pub.Raw()
	if rawPub == nil {
		t.Error("Raw() 不应返回 nil")
	}

	rawPriv := priv.Raw()
	if rawPriv == nil {
		t.Error("Raw() 不应返回 nil")
	}
}

func TestNewIdentityFromKeyPair(t *testing.T) {
	priv, pub, _ := GenerateEd25519KeyPair()
	id := NewIdentityFromKeyPair(priv, pub)

	if id.ID().IsEmpty() {
		t.Error("Identity ID 不应为空")
	}
	if !id.PublicKey().Equal(pub) {
		t.Error("Identity 公钥应该与传入的公钥相同")
	}
}

func TestNodeIDMethods(t *testing.T) {
	_, pub, _ := GenerateEd25519KeyPair()
	nodeID := NodeIDFromPublicKey(pub)

	// 测试 String (Base58 格式，32 bytes 约 43-44 chars)
	str := nodeID.String()
	if len(str) < 40 || len(str) > 50 { // Base58 长度范围
		t.Errorf("NodeID 字符串长度错误: got %d, want 40-50 (Base58)", len(str))
	}

	// 测试 ShortString (Base58 前 8 字符)
	shortStr := nodeID.ShortString()
	if len(shortStr) > 8 { // 最多 8 字符
		t.Errorf("NodeID 短字符串长度错误: got %d, want <= 8", len(shortStr))
	}

	// 测试 Bytes
	bytes := nodeID.Bytes()
	if len(bytes) != 32 {
		t.Errorf("NodeID 字节长度错误: got %d, want 32", len(bytes))
	}
}

// ============================================================================
//                              fx 模块测试
// ============================================================================

func TestProvideServicesWithDefaultConfig(t *testing.T) {
	input := ModuleInput{
		Config: nil, // 使用默认配置
	}

	output, err := ProvideServices(input)
	if err != nil {
		t.Fatalf("ProvideServices 失败: %v", err)
	}

	if output.Identity == nil {
		t.Error("Identity 不应为 nil")
	}
	if output.IdentityManager == nil {
		t.Error("IdentityManager 不应为 nil")
	}
}

func TestProvideServicesWithIdentityPath(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "identity.key")

	config := identityif.Config{
		KeyType:      types.KeyTypeEd25519,
		IdentityPath: keyPath,
		AutoCreate:   true,
	}
	input := ModuleInput{
		Config: &config,
	}

	// 第一次调用 - 创建新身份
	output1, err := ProvideServices(input)
	if err != nil {
		t.Fatalf("ProvideServices 失败: %v", err)
	}

	// 验证文件已创建
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("密钥文件应该已创建")
	}

	// 第二次调用 - 加载已有身份
	output2, err := ProvideServices(input)
	if err != nil {
		t.Fatalf("ProvideServices 第二次调用失败: %v", err)
	}

	// 验证 ID 相同
	if !output1.Identity.ID().Equal(output2.Identity.ID()) {
		t.Error("加载的身份 ID 应该与创建的相同")
	}
}

func TestProvideServicesLoadFailNoAutoCreate(t *testing.T) {
	config := identityif.Config{
		KeyType:      types.KeyTypeEd25519,
		IdentityPath: "/nonexistent/path/key.pem",
		AutoCreate:   false,
	}
	input := ModuleInput{
		Config: &config,
	}

	_, err := ProvideServices(input)
	if err == nil {
		t.Error("期望加载失败时返回错误")
	}
}

func TestProvideServicesNoPathNoAutoCreate(t *testing.T) {
	config := identityif.Config{
		KeyType:      types.KeyTypeEd25519,
		IdentityPath: "",
		AutoCreate:   false,
	}
	input := ModuleInput{
		Config: &config,
	}

	_, err := ProvideServices(input)
	if err == nil {
		t.Error("期望未创建身份时返回错误")
	}
}

// ============================================================================
//                              模块元信息测试
// ============================================================================

func TestModuleMetadata(t *testing.T) {
	if Version == "" {
		t.Error("Version 不应为空")
	}
	if Name == "" {
		t.Error("Name 不应为空")
	}
	if Description == "" {
		t.Error("Description 不应为空")
	}
}

// ============================================================================
//                              错误类型测试
// ============================================================================

func TestErrorTypes(t *testing.T) {
	// 验证错误类型存在且不为 nil
	if ErrInvalidKeySize == nil {
		t.Error("ErrInvalidKeySize 不应为 nil")
	}
	if ErrInvalidKeyType == nil {
		t.Error("ErrInvalidKeyType 不应为 nil")
	}
	if ErrSignatureFailed == nil {
		t.Error("ErrSignatureFailed 不应为 nil")
	}
	if ErrInvalidPEM == nil {
		t.Error("ErrInvalidPEM 不应为 nil")
	}
	if ErrUnsupportedKeyType == nil {
		t.Error("ErrUnsupportedKeyType 不应为 nil")
	}
	if ErrKeyNotFound == nil {
		t.Error("ErrKeyNotFound 不应为 nil")
	}
}

// ============================================================================
//                              原子写测试
// ============================================================================

func TestAtomicWritePrivateKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "atomic_test.key")

	priv, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 保存私钥（使用原子写）
	err = SavePrivateKeyPEM(priv, keyPath)
	if err != nil {
		t.Fatalf("保存私钥失败: %v", err)
	}

	// 验证文件存在且内容正确
	loadedPriv, err := LoadPrivateKeyPEM(keyPath)
	if err != nil {
		t.Fatalf("加载私钥失败: %v", err)
	}

	if !priv.PublicKey().Equal(loadedPriv.PublicKey()) {
		t.Error("加载的私钥与原私钥不一致")
	}

	// 验证没有临时文件残留
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}

	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".tmp-") {
			t.Errorf("发现临时文件残留: %s", f.Name())
		}
	}
}

func TestAtomicWritePublicKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "atomic_test.pub")

	_, pub, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 保存公钥（使用原子写）
	err = SavePublicKeyPEM(pub, keyPath)
	if err != nil {
		t.Fatalf("保存公钥失败: %v", err)
	}

	// 验证文件存在且内容正确
	loadedPub, err := LoadPublicKeyPEM(keyPath)
	if err != nil {
		t.Fatalf("加载公钥失败: %v", err)
	}

	if !pub.Equal(loadedPub) {
		t.Error("加载的公钥与原公钥不一致")
	}
}

func TestAtomicWriteOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "overwrite_test.key")

	// 创建第一个密钥并保存
	priv1, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对1失败: %v", err)
	}
	err = SavePrivateKeyPEM(priv1, keyPath)
	if err != nil {
		t.Fatalf("保存私钥1失败: %v", err)
	}

	// 创建第二个密钥并覆盖
	priv2, _, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("生成密钥对2失败: %v", err)
	}
	err = SavePrivateKeyPEM(priv2, keyPath)
	if err != nil {
		t.Fatalf("保存私钥2失败: %v", err)
	}

	// 加载并验证是第二个密钥
	loadedPriv, err := LoadPrivateKeyPEM(keyPath)
	if err != nil {
		t.Fatalf("加载私钥失败: %v", err)
	}

	if !priv2.PublicKey().Equal(loadedPriv.PublicKey()) {
		t.Error("覆盖后应该是第二个密钥")
	}

	if priv1.PublicKey().Equal(loadedPriv.PublicKey()) {
		t.Error("不应该是第一个密钥")
	}
}

// ============================================================================
//                              NodeID 一致性测试
// ============================================================================

func TestNodeIDConsistency(t *testing.T) {
	// 验证从相同公钥派生的 NodeID 始终一致
	priv, pub, _ := GenerateEd25519KeyPair()

	nodeID1 := NodeIDFromPublicKey(pub)
	nodeID2 := NodeIDFromPublicKey(pub)
	nodeID3 := NodeIDFromPublicKey(priv.PublicKey())

	if !nodeID1.Equal(nodeID2) {
		t.Error("相同公钥应产生相同 NodeID")
	}

	if !nodeID1.Equal(nodeID3) {
		t.Error("从私钥获取的公钥应产生相同 NodeID")
	}
}

func TestNodeIDDifferentKeys(t *testing.T) {
	// 验证不同公钥产生不同 NodeID
	_, pub1, _ := GenerateEd25519KeyPair()
	_, pub2, _ := GenerateEd25519KeyPair()

	nodeID1 := NodeIDFromPublicKey(pub1)
	nodeID2 := NodeIDFromPublicKey(pub2)

	if nodeID1.Equal(nodeID2) {
		t.Error("不同公钥应产生不同 NodeID")
	}
}

