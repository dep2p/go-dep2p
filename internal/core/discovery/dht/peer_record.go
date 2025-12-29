// Package dht 提供分布式哈希表实现
package dht

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"

	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Key 命名规范
// ============================================================================

const (
	// PeerRecordKeyPrefix 节点地址记录 key 前缀
	// 格式：dep2p/v1/sys/peer/<NodeID>
	PeerRecordKeyPrefix = "dep2p/v1/sys/peer/"

	// ProviderKeyPrefix Provider 记录 key 前缀（sys 域）
	// 格式：dep2p/v1/sys/<namespace>
	ProviderKeyPrefix = "dep2p/v1/sys/"

	// RealmKeyPrefix Realm 域 key 前缀
	// 格式：dep2p/v1/realm/<realmID>/<namespace>
	RealmKeyPrefix = "dep2p/v1/realm/"

	// DefaultPeerRecordTTL 默认 PeerRecord TTL
	DefaultPeerRecordTTL = 1 * time.Hour

	// MaxPeerRecordTTL 最大 PeerRecord TTL
	MaxPeerRecordTTL = 24 * time.Hour

	// SysNamespacePrefix 系统命名空间前缀
	// 用于在 namespace 中显式指定 sys 域（如 "sys:relay"）
	SysNamespacePrefix = "sys:"
)

// KeyScope 表示 key 的作用域
type KeyScope int

const (
	// KeyScopeSys 系统域（默认）
	// Key 格式: dep2p/v1/sys/<namespace>
	KeyScopeSys KeyScope = iota

	// KeyScopeRealm Realm 域
	// Key 格式: dep2p/v1/realm/<realmID>/<namespace>
	KeyScopeRealm
)

// String 返回 KeyScope 的字符串表示
func (s KeyScope) String() string {
	switch s {
	case KeyScopeSys:
		return "sys"
	case KeyScopeRealm:
		return "realm"
	default:
		return "unknown"
	}
}

// ParsedNamespace 解析后的命名空间
type ParsedNamespace struct {
	// Scope 作用域
	Scope KeyScope

	// Namespace 归一化后的命名空间（不含前缀）
	Namespace string
}

// NormalizeNamespace 归一化 namespace 并提取 scope
//
// 规则：
// 1. 以 "sys:" 前缀开头 → 强制 KeyScopeSys，移除前缀
// 2. 以 "sys/" 开头（错误用法）→ 强制 KeyScopeSys，移除前缀并记录警告
// 3. 其他 → 默认 KeyScopeSys（保持兼容性）
//
// 示例：
//   - "sys:relay" → {KeyScopeSys, "relay"}
//   - "relay" → {KeyScopeSys, "relay"}
//   - "sys/relay" → {KeyScopeSys, "relay"} (并记录警告，防止双前缀)
func NormalizeNamespace(namespace string) ParsedNamespace {
	// 移除前后空白
	namespace = strings.TrimSpace(namespace)

	// 规则1：显式 "sys:" 前缀
	if strings.HasPrefix(namespace, SysNamespacePrefix) {
		return ParsedNamespace{
			Scope:     KeyScopeSys,
			Namespace: strings.TrimPrefix(namespace, SysNamespacePrefix),
		}
	}

	// 规则2：错误的 "sys/" 前缀（防止双前缀陷阱）
	if strings.HasPrefix(namespace, "sys/") {
		// 记录警告（实际日志在调用方）
		return ParsedNamespace{
			Scope:     KeyScopeSys,
			Namespace: strings.TrimPrefix(namespace, "sys/"),
		}
	}

	// 规则3：默认 sys 域
	return ParsedNamespace{
		Scope:     KeyScopeSys,
		Namespace: namespace,
	}
}

// BuildProviderKey 根据 scope 和 realmID 构建 provider key
//
// 格式：
//   - KeyScopeSys: dep2p/v1/sys/<namespace>
//   - KeyScopeRealm: dep2p/v1/realm/<realmID>/<namespace>
//
// 注意：
//   - namespace 应该是已归一化的（通过 NormalizeNamespace）
//   - 对于 KeyScopeRealm，realmID 不能为空
func BuildProviderKey(scope KeyScope, realmID string, namespace string) string {
	switch scope {
	case KeyScopeRealm:
		if realmID == "" {
			// 回退到 sys 域
			return ProviderKeyPrefix + namespace
		}
		return RealmKeyPrefix + realmID + "/" + namespace
	default:
		return ProviderKeyPrefix + namespace
	}
}

// BuildProviderKeyWithParsed 使用解析后的 namespace 构建 key
//
// 这是推荐的方式：先用 NormalizeNamespace 解析，再用此函数构建 key
func BuildProviderKeyWithParsed(parsed ParsedNamespace, realmID string) string {
	return BuildProviderKey(parsed.Scope, realmID, parsed.Namespace)
}

// ============================================================================
//                              SignedPeerRecord
// ============================================================================

// SignedPeerRecord 签名的节点地址记录
//
// 用于在 DHT 中发布和验证节点地址信息。
// 记录由节点私钥签名，确保地址信息不被篡改。
//
// T3 修复：添加 PubKeyBytes 字段，使记录自包含可验证。
// P1 修复：添加 KeyType 字段，支持多密钥类型（Ed25519/ECDSA/RSA）。
type SignedPeerRecord struct {
	// NodeID 节点 ID
	NodeID types.NodeID `json:"node_id"`

	// Addrs 可拨号地址列表
	Addrs []string `json:"addrs"`

	// Seqno 序列号（单调递增，用于防止回滚攻击）
	Seqno uint64 `json:"seqno"`

	// Timestamp 创建时间戳（Unix 纳秒）
	Timestamp int64 `json:"timestamp"`

	// TTL 生存时间（秒）
	TTL uint32 `json:"ttl"`

	// KeyType 密钥类型（P1：支持多密钥类型）
	// 0 = Ed25519（默认，向后兼容）
	// 1 = ECDSA P-256
	// 2 = RSA
	KeyType uint8 `json:"key_type,omitempty"`

	// PubKeyBytes 公钥字节（T3：用于签名验证）
	// 格式：与 Identity.PublicKey().Bytes() 一致
	// - Ed25519: 32 字节原始公钥
	// - ECDSA: X.509 PKIX DER 编码
	// - RSA: X.509 PKIX DER 编码
	PubKeyBytes []byte `json:"pub_key,omitempty"`

	// Signature 签名（对上述字段的摘要签名）
	Signature []byte `json:"signature"`
}

// KeyType 常量
const (
	KeyTypeEd25519 uint8 = 0 // Ed25519（默认）
	KeyTypeECDSA   uint8 = 1 // ECDSA P-256
	KeyTypeRSA     uint8 = 2 // RSA
)

// NewSignedPeerRecord 创建签名的节点地址记录
//
// identity: 节点身份（用于签名，使用 DHT 内部的 Identity 接口）
// addrs: 可拨号地址列表
// seqno: 序列号
// ttl: 生存时间
//
// T3 修复：现在需要 IdentityWithPubKey 以包含公钥
// P1 修复：自动检测密钥类型并设置 KeyType 字段
func NewSignedPeerRecord(id IdentityWithPubKey, addrs []string, seqno uint64, ttl time.Duration) (*SignedPeerRecord, error) {
	if id == nil {
		return nil, errors.New("identity is required")
	}

	// 限制 TTL
	if ttl <= 0 {
		ttl = DefaultPeerRecordTTL
	}
	if ttl > MaxPeerRecordTTL {
		ttl = MaxPeerRecordTTL
	}

	pubKeyBytes := id.PubKeyBytes()
	keyType := detectKeyType(pubKeyBytes)

	record := &SignedPeerRecord{
		NodeID:      id.ID(),
		Addrs:       addrs,
		Seqno:       seqno,
		Timestamp:   time.Now().UnixNano(),
		TTL:         uint32(ttl.Seconds()),
		KeyType:     keyType,
		PubKeyBytes: pubKeyBytes,
	}

	// 计算签名
	digest := record.Digest()
	signature, err := id.Sign(digest)
	if err != nil {
		return nil, err
	}
	record.Signature = signature

	return record, nil
}

// detectKeyType 根据公钥字节检测密钥类型
func detectKeyType(pubKeyBytes []byte) uint8 {
	// Ed25519: 固定 32 字节
	if len(pubKeyBytes) == ed25519.PublicKeySize {
		return KeyTypeEd25519
	}

	// 尝试解析为 X.509 PKIX 格式（ECDSA/RSA）
	pub, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		// 默认 Ed25519
		return KeyTypeEd25519
	}

	switch pub.(type) {
	case *ecdsa.PublicKey:
		return KeyTypeECDSA
	case *rsa.PublicKey:
		return KeyTypeRSA
	default:
		return KeyTypeEd25519
	}
}

// Digest 计算记录摘要（用于签名/验证）
func (r *SignedPeerRecord) Digest() []byte {
	h := sha256.New()

	// NodeID
	h.Write(r.NodeID[:])

	// Addrs count + Addrs
	_ = binary.Write(h, binary.BigEndian, uint16(len(r.Addrs))) //nolint:gosec // G115: 地址数量由协议限制
	for _, addr := range r.Addrs {
		_ = binary.Write(h, binary.BigEndian, uint16(len(addr))) //nolint:gosec // G115: 地址长度由协议限制
		h.Write([]byte(addr))
	}

	// Seqno
	_ = binary.Write(h, binary.BigEndian, r.Seqno)

	// Timestamp
	_ = binary.Write(h, binary.BigEndian, r.Timestamp)

	// TTL
	_ = binary.Write(h, binary.BigEndian, r.TTL)

	return h.Sum(nil)
}

// Verify 验证记录签名
//
// pubKey: 发布者的公钥（应与 NodeID 对应）
func (r *SignedPeerRecord) Verify(pubKey identityif.PublicKey) (bool, error) {
	if pubKey == nil {
		return false, errors.New("public key is required")
	}

	// 注意：NodeID 与公钥的匹配验证应在更高层进行
	// 这里只验证签名本身的有效性

	// 验证签名
	digest := r.Digest()
	return pubKey.Verify(digest, r.Signature)
}

// VerifySelf 使用记录内嵌的公钥验证签名
//
// T3 修复：自包含验证，不需要外部公钥
// P1 修复：支持多密钥类型（Ed25519/ECDSA/RSA）
//
// 验证步骤：
// 1. 检查 PubKeyBytes 非空
// 2. SHA256(PubKeyBytes) == NodeID（验证公钥与 NodeID 匹配）
// 3. 根据 KeyType 选择验证方法
func (r *SignedPeerRecord) VerifySelf() error {
	if len(r.PubKeyBytes) == 0 {
		return errors.New("record missing public key")
	}

	// 验证公钥与 NodeID 匹配
	// NodeID = SHA256(PubKeyBytes)
	expectedNodeID := sha256.Sum256(r.PubKeyBytes)
	if !bytes.Equal(expectedNodeID[:], r.NodeID[:]) {
		return errors.New("public key does not match node ID")
	}

	digest := r.Digest()

	// 根据 KeyType 选择验证方法
	switch r.KeyType {
	case KeyTypeEd25519:
		// Ed25519: 32 字节公钥
		if len(r.PubKeyBytes) != ed25519.PublicKeySize {
			return errors.New("invalid Ed25519 public key length (expected 32 bytes)")
		}
		if !verifyEd25519(r.PubKeyBytes, digest, r.Signature) {
			return errors.New("invalid Ed25519 signature")
		}

	case KeyTypeECDSA:
		// ECDSA: X.509 PKIX DER 编码
		if !verifyECDSA(r.PubKeyBytes, digest, r.Signature) {
			return errors.New("invalid ECDSA signature")
		}

	case KeyTypeRSA:
		// RSA: X.509 PKIX DER 编码
		if !verifyRSA(r.PubKeyBytes, digest, r.Signature) {
			return errors.New("invalid RSA signature")
		}

	default:
		// 向后兼容：未知类型尝试 Ed25519（旧记录可能没有 KeyType 字段）
		if len(r.PubKeyBytes) == ed25519.PublicKeySize {
			if !verifyEd25519(r.PubKeyBytes, digest, r.Signature) {
				return errors.New("invalid signature (fallback Ed25519)")
			}
		} else {
			return errors.New("unsupported key type")
		}
	}

	return nil
}

// verifyEd25519 使用 Ed25519 验证签名
func verifyEd25519(pubKey, message, sig []byte) bool {
	// Ed25519 签名长度为 64 字节
	if len(sig) != ed25519.SignatureSize {
		return false
	}
	// Ed25519 公钥长度为 32 字节
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	// 使用标准库验证
	return ed25519.Verify(pubKey, message, sig)
}

// verifyECDSA 使用 ECDSA 验证签名
//
// P1 修复：支持 ECDSA P-256 签名验证
// pubKey: X.509 PKIX DER 编码的 ECDSA 公钥
// message: 原始消息（函数内部会计算 SHA256 哈希）
// sig: ASN.1 DER 编码的签名，或 r||s 格式（64 字节）
func verifyECDSA(pubKeyBytes, message, sig []byte) bool {
	// 解析 X.509 PKIX 公钥
	pub, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		return false
	}

	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return false
	}

	// 计算消息的 SHA256 哈希
	hash := sha256.Sum256(message)

	// 尝试 ASN.1 DER 格式验证
	if ecdsa.VerifyASN1(ecdsaPub, hash[:], sig) {
		return true
	}

	// 回退：尝试 r||s 格式（P-256: 64 字节 = 32 + 32）
	if len(sig) == 64 {
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		return ecdsa.Verify(ecdsaPub, hash[:], r, s)
	}

	return false
}

// verifyRSA 使用 RSA 验证签名
//
// P1 修复：支持 RSA PKCS#1 v1.5 签名验证
// pubKey: X.509 PKIX DER 编码的 RSA 公钥
// message: 原始消息（函数内部会计算 SHA256 哈希）
// sig: RSA 签名
func verifyRSA(pubKeyBytes, message, sig []byte) bool {
	// 解析 X.509 PKIX 公钥
	pub, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		return false
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return false
	}

	// 计算消息的 SHA256 哈希
	hash := sha256.Sum256(message)

	// 使用 PKCS#1 v1.5 验证
	err = rsa.VerifyPKCS1v15(rsaPub, 0, hash[:], sig)
	return err == nil
}

// IsExpired 检查记录是否过期
func (r *SignedPeerRecord) IsExpired() bool {
	created := time.Unix(0, r.Timestamp)
	return time.Since(created) > time.Duration(r.TTL)*time.Second
}

// IsNewerThan 比较两个记录的新旧
//
// 优先比较 Seqno，若相等则比较 Timestamp
func (r *SignedPeerRecord) IsNewerThan(other *SignedPeerRecord) bool {
	if other == nil {
		return true
	}
	if r.Seqno != other.Seqno {
		return r.Seqno > other.Seqno
	}
	return r.Timestamp > other.Timestamp
}

// Key 返回 DHT 存储 key
func (r *SignedPeerRecord) Key() string {
	return PeerRecordKeyPrefix + r.NodeID.String()
}

// ============================================================================
//                              编解码
// ============================================================================

// Encode 编码记录为字节数组
func (r *SignedPeerRecord) Encode() ([]byte, error) {
	return json.Marshal(r)
}

// DecodeSignedPeerRecord 从字节数组解码记录
func DecodeSignedPeerRecord(data []byte) (*SignedPeerRecord, error) {
	var record SignedPeerRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// EncodeBinary 二进制编码（更紧凑）
func (r *SignedPeerRecord) EncodeBinary() []byte {
	var buf bytes.Buffer

	// NodeID (32 bytes)
	buf.Write(r.NodeID[:])

	// Addrs count (2 bytes) + Addrs
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(r.Addrs))) //nolint:gosec // G115: 地址数量由协议限制
	for _, addr := range r.Addrs {
		_ = binary.Write(&buf, binary.BigEndian, uint16(len(addr))) //nolint:gosec // G115: 地址长度由协议限制
		buf.WriteString(addr)
	}

	// Seqno (8 bytes)
	_ = binary.Write(&buf, binary.BigEndian, r.Seqno)

	// Timestamp (8 bytes)
	_ = binary.Write(&buf, binary.BigEndian, r.Timestamp)

	// TTL (4 bytes)
	_ = binary.Write(&buf, binary.BigEndian, r.TTL)

	// Signature length (2 bytes) + Signature
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(r.Signature))) //nolint:gosec // G115: 签名长度由密码算法限制
	buf.Write(r.Signature)

	return buf.Bytes()
}

// DecodeSignedPeerRecordBinary 从二进制数据解码
func DecodeSignedPeerRecordBinary(data []byte) (*SignedPeerRecord, error) {
	if len(data) < 54 { // 最小长度: 32 + 2 + 8 + 8 + 4 = 54
		return nil, errors.New("data too short")
	}

	r := &SignedPeerRecord{}
	buf := bytes.NewReader(data)

	// NodeID
	if _, err := buf.Read(r.NodeID[:]); err != nil {
		return nil, err
	}

	// Addrs count
	var addrCount uint16
	if err := binary.Read(buf, binary.BigEndian, &addrCount); err != nil {
		return nil, err
	}

	// 防止 OOM
	if addrCount > 100 {
		return nil, errors.New("too many addresses")
	}

	// Addrs
	r.Addrs = make([]string, addrCount)
	for i := uint16(0); i < addrCount; i++ {
		var addrLen uint16
		if err := binary.Read(buf, binary.BigEndian, &addrLen); err != nil {
			return nil, err
		}
		if addrLen > 1024 {
			return nil, errors.New("address too long")
		}
		addrBytes := make([]byte, addrLen)
		if _, err := buf.Read(addrBytes); err != nil {
			return nil, err
		}
		r.Addrs[i] = string(addrBytes)
	}

	// Seqno
	if err := binary.Read(buf, binary.BigEndian, &r.Seqno); err != nil {
		return nil, err
	}

	// Timestamp
	if err := binary.Read(buf, binary.BigEndian, &r.Timestamp); err != nil {
		return nil, err
	}

	// TTL
	if err := binary.Read(buf, binary.BigEndian, &r.TTL); err != nil {
		return nil, err
	}

	// Signature length
	var sigLen uint16
	if err := binary.Read(buf, binary.BigEndian, &sigLen); err != nil {
		return nil, err
	}
	if sigLen > 512 {
		return nil, errors.New("signature too long")
	}

	// Signature
	r.Signature = make([]byte, sigLen)
	if _, err := buf.Read(r.Signature); err != nil {
		return nil, err
	}

	return r, nil
}

// ============================================================================
//                              验证规则
// ============================================================================

// PeerRecordValidator PeerRecord 验证器
type PeerRecordValidator struct {
	// 已知公钥缓存（可选，用于避免重复获取公钥）
	pubKeyCache map[types.NodeID]identityif.PublicKey
}

// NewPeerRecordValidator 创建验证器
func NewPeerRecordValidator() *PeerRecordValidator {
	return &PeerRecordValidator{
		pubKeyCache: make(map[types.NodeID]identityif.PublicKey),
	}
}

// Validate 验证 PeerRecord
//
// 验证规则：
// 1. 签名有效（由 NodeID 对应私钥签名）
// 2. 记录未过期
// 3. 地址格式有效（基本检查）
// 4. TTL 在允许范围内
func (v *PeerRecordValidator) Validate(record *SignedPeerRecord, pubKey identityif.PublicKey) error {
	if record == nil {
		return errors.New("record is nil")
	}

	// 验证 TTL 范围
	if record.TTL == 0 || record.TTL > uint32(MaxPeerRecordTTL.Seconds()) {
		return errors.New("invalid TTL")
	}

	// 验证过期
	if record.IsExpired() {
		return errors.New("record expired")
	}

	// 验证地址数量
	if len(record.Addrs) == 0 {
		return errors.New("no addresses")
	}
	if len(record.Addrs) > 100 {
		return errors.New("too many addresses")
	}

	// 验证签名
	if pubKey != nil {
		valid, err := record.Verify(pubKey)
		if err != nil {
			return err
		}
		if !valid {
			return errors.New("invalid signature")
		}
	}

	return nil
}

// ShouldReplace 判断新记录是否应该替换旧记录
//
// 替换规则：
// 1. 新记录 Seqno 更大
// 2. Seqno 相等但 Timestamp 更新
func (v *PeerRecordValidator) ShouldReplace(existing, incoming *SignedPeerRecord) bool {
	if existing == nil {
		return true
	}
	return incoming.IsNewerThan(existing)
}

