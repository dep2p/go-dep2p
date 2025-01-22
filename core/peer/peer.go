// Package peer 实现了用于表示 dep2p 网络中对等节点的对象
package peer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dep2p/go-dep2p/cid"
	ic "github.com/dep2p/go-dep2p/core/crypto"
	mc "github.com/dep2p/go-dep2p/multiformats/multicodec"
	mh "github.com/dep2p/go-dep2p/multiformats/multihash"
	logging "github.com/dep2p/log"
	b58 "github.com/mr-tron/base58/base58"
)

var (
	// ErrEmptyPeerID 表示空的对等节点 ID 错误
	ErrEmptyPeerID = errors.New("空的对等节点 ID")
	// ErrNoPublicKey 表示对等节点 ID 中未嵌入公钥的错误
	ErrNoPublicKey = errors.New("对等节点 ID 中未嵌入公钥")
)

var log = logging.Logger("core-peer")

// AdvancedEnableInlining 启用自动将短于 42 字节的密钥内联到对等节点 ID 中(使用"identity" multihash 函数)
//
// 警告：此标志在将来可能会设置为 false，并最终被移除，改为使用密钥本身指定的哈希函数
//
// 除非你知道自己在做什么，否则不要更改此标志
//
// 目前默认为 true 以保持向后兼容性，但在确定升级路径后可能会默认设置为 false
var AdvancedEnableInlining = true

// maxInlineKeyLength 定义可以内联的最大密钥长度
const maxInlineKeyLength = 42

// ID 是 dep2p 对等节点标识
//
// 对等节点 ID 通过对节点的公钥进行哈希并将哈希输出编码为 multihash 来派生
// 详见 IDFromPublicKey
type ID string

// Loggable 返回格式化的对等节点 ID 字符串，用于日志记录
// 返回值：
//   - map[string]interface{}: 包含格式化后的对等节点 ID 的映射
func (id ID) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peerID": id.String(),
	}
}

// String 将对等节点 ID 转换为字符串表示
// 返回值：
//   - string: 对等节点 ID 的 base58 编码字符串
func (id ID) String() string {
	return b58.Encode([]byte(id))
}

// ShortString 打印对等节点 ID 的简短形式
//
// TODO(brian): 确保 ID 生成的正确性，并通过只暴露安全生成 ID 的函数来强制执行
// 这样代码库中的任何 peer.ID 类型都可以确保是正确的
// 返回值：
//   - string: 对等节点 ID 的简短字符串表示
func (id ID) ShortString() string {
	pid := id.String()
	if len(pid) <= 10 {
		return fmt.Sprintf("<peer.ID %s>", pid)
	}
	return fmt.Sprintf("<peer.ID %s*%s>", pid[:2], pid[len(pid)-6:])
}

// MatchesPrivateKey 测试此 ID 是否由私钥 sk 派生
// 参数：
//   - sk: ic.PrivKey 要测试的私钥
//
// 返回值：
//   - bool: 如果 ID 与私钥匹配返回 true，否则返回 false
func (id ID) MatchesPrivateKey(sk ic.PrivKey) bool {
	return id.MatchesPublicKey(sk.GetPublic())
}

// MatchesPublicKey 测试此 ID 是否由公钥 pk 派生
// 参数：
//   - pk: ic.PubKey 要测试的公钥
//
// 返回值：
//   - bool: 如果 ID 与公钥匹配返回 true，否则返回 false
func (id ID) MatchesPublicKey(pk ic.PubKey) bool {
	oid, err := IDFromPublicKey(pk)
	if err != nil {
		return false
	}
	return oid == id
}

// ExtractPublicKey 尝试从 ID 中提取公钥
//
// 如果对等节点 ID 看起来有效但无法提取公钥，此方法返回 ErrNoPublicKey
// 返回值：
//   - ic.PubKey: 提取的公钥
//   - error: 如果发生错误，返回错误信息
func (id ID) ExtractPublicKey() (ic.PubKey, error) {
	decoded, err := mh.Decode([]byte(id))
	if err != nil {
		return nil, err
	}
	if decoded.Code != mh.IDENTITY {
		return nil, ErrNoPublicKey
	}
	pk, err := ic.UnmarshalPublicKey(decoded.Digest)
	if err != nil {
		return nil, err
	}
	return pk, nil
}

// Validate 检查 ID 是否为空
// 返回值：
//   - error: 如果 ID 为空返回 ErrEmptyPeerID，否则返回 nil
func (id ID) Validate() error {
	if id == ID("") {
		return ErrEmptyPeerID
	}

	return nil
}

// IDFromBytes 将字节切片转换为 ID 类型，并验证该值是否为 multihash
// 参数：
//   - b: []byte 要转换的字节切片
//
// 返回值：
//   - ID: 转换后的对等节点 ID
//   - error: 如果发生错误，返回错误信息
func IDFromBytes(b []byte) (ID, error) {
	if _, err := mh.Cast(b); err != nil {
		return ID(""), err
	}
	return ID(b), nil
}

// Decode 接受编码的对等节点 ID 并在输入有效时返回解码后的 ID
//
// 编码的对等节点 ID 可以是密钥的 CID 或原始 multihash (identity 或 sha256-256)
// 参数：
//   - s: string 要解码的对等节点 ID 字符串
//
// 返回值：
//   - ID: 解码后的对等节点 ID
//   - error: 如果发生错误，返回错误信息
func Decode(s string) (ID, error) {
	if strings.HasPrefix(s, "Qm") || strings.HasPrefix(s, "1") {
		// base58 编码的 sha256 或 identity multihash
		m, err := mh.FromB58String(s)
		if err != nil {
			log.Debugf("解析peer ID失败: %s", err)
			return "", err
		}
		return ID(m), nil
	}

	c, err := cid.Decode(s)
	if err != nil {
		log.Debugf("解析peer ID失败: %s", err)
		return "", err
	}
	return FromCid(c)
}

// FromCid 将 CID 转换为对等节点 ID
// 参数：
//   - c: cid.Cid 要转换的 CID
//
// 返回值：
//   - ID: 转换后的对等节点 ID
//   - error: 如果发生错误，返回错误信息
func FromCid(c cid.Cid) (ID, error) {
	code := mc.Code(c.Type())
	if code != mc.Libp2pKey {
		return "", fmt.Errorf("can't convert CID of type %q to a peer ID", code)
	}
	return ID(c.Hash()), nil
}

// ToCid 将对等节点 ID 编码为公钥的 CID
//
// 如果对等节点 ID 无效(例如为空)，将返回空 CID
// 参数：
//   - id: ID 要编码的对等节点 ID
//
// 返回值：
//   - cid.Cid: 编码后的 CID
func ToCid(id ID) cid.Cid {
	m, err := mh.Cast([]byte(id))
	if err != nil {
		return cid.Cid{}
	}
	return cid.NewCidV1(cid.Libp2pKey, m)
}

// IDFromPublicKey 返回与公钥 pk 对应的对等节点 ID
// 参数：
//   - pk: ic.PubKey 用于生成对等节点 ID 的公钥
//
// 返回值：
//   - ID: 生成的对等节点 ID
//   - error: 如果发生错误，返回错误信息
func IDFromPublicKey(pk ic.PubKey) (ID, error) {
	b, err := ic.MarshalPublicKey(pk)
	if err != nil {
		return "", err
	}
	var alg uint64 = mh.SHA2_256
	if AdvancedEnableInlining && len(b) <= maxInlineKeyLength {
		alg = mh.IDENTITY
	}
	hash, _ := mh.Sum(b, alg, -1)
	return ID(hash), nil
}

// IDFromPrivateKey 返回与私钥 sk 对应的对等节点 ID
// 参数：
//   - sk: ic.PrivKey 用于生成对等节点 ID 的私钥
//
// 返回值：
//   - ID: 生成的对等节点 ID
//   - error: 如果发生错误，返回错误信息
func IDFromPrivateKey(sk ic.PrivKey) (ID, error) {
	return IDFromPublicKey(sk.GetPublic())
}

// IDSlice 用于对等节点排序的切片类型
type IDSlice []ID

// Len 返回切片长度
// 返回值：
//   - int: 切片中元素的数量
func (es IDSlice) Len() int { return len(es) }

// Swap 交换切片中的两个元素
// 参数：
//   - i: int 第一个元素的索引
//   - j: int 第二个元素的索引
func (es IDSlice) Swap(i, j int) { es[i], es[j] = es[j], es[i] }

// Less 比较切片中的两个元素
// 参数：
//   - i: int 第一个元素的索引
//   - j: int 第二个元素的索引
//
// 返回值：
//   - bool: 如果第一个元素小于第二个元素返回 true
func (es IDSlice) Less(i, j int) bool { return string(es[i]) < string(es[j]) }

// String 返回切片的字符串表示
// 返回值：
//   - string: 以逗号分隔的对等节点 ID 字符串
func (es IDSlice) String() string {
	peersStrings := make([]string, len(es))
	for i, id := range es {
		peersStrings[i] = id.String()
	}
	return strings.Join(peersStrings, ", ")
}
