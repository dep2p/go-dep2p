package dep2pwebrtc

import (
	"crypto"
	"fmt"
	"net"
	"strings"

	"github.com/dep2p/go-dep2p/multiformats/multihash"
)

// clientSDP 定义了一个SDP格式字符串,用于从传入的STUN消息推断客户端的SDP offer
// 由于指纹验证被禁用而改用噪声握手,因此用于渲染客户端SDP的指纹是任意的
// 最大消息大小固定为16384字节
const clientSDP = `v=0
o=- 0 0 IN %[1]s %[2]s
s=-
c=IN %[1]s %[2]s
t=0 0

m=application %[3]d UDP/DTLS/SCTP webrtc-datachannel
a=mid:0
a=ice-options:ice2
a=ice-ufrag:%[4]s
a=ice-pwd:%[4]s
a=fingerprint:sha-256 ba:78:16:bf:8f:01:cf:ea:41:41:40:de:5d:ae:22:23:b0:03:61:a3:96:17:7a:9c:b4:10:ff:61:f2:00:15:ad
a=setup:actpass
a=sctp-port:5000
a=max-message-size:16384
`

// createClientSDP 创建客户端的SDP字符串
// 参数:
//   - addr: *net.UDPAddr UDP地址对象
//   - ufrag: string ICE用户片段
//
// 返回值:
//   - string 格式化后的SDP字符串
func createClientSDP(addr *net.UDPAddr, ufrag string) string {
	// 根据IP地址类型确定IP版本
	ipVersion := "IP4"
	if addr.IP.To4() == nil {
		ipVersion = "IP6"
	}
	// 使用格式化字符串生成SDP
	return fmt.Sprintf(
		clientSDP,
		ipVersion,
		addr.IP,
		addr.Port,
		ufrag,
	)
}

// serverSDP 定义了一个SDP格式字符串,用于拨号器根据提供的多地址和本地设置的ICE凭证推断服务器的SDP应答
// 最大消息大小固定为16384字节
const serverSDP = `v=0
o=- 0 0 IN %[1]s %[2]s
s=-
t=0 0
a=ice-lite
m=application %[3]d UDP/DTLS/SCTP webrtc-datachannel
c=IN %[1]s %[2]s
a=mid:0
a=ice-options:ice2
a=ice-ufrag:%[4]s
a=ice-pwd:%[4]s
a=fingerprint:%[5]s

a=setup:passive
a=sctp-port:5000
a=max-message-size:16384
a=candidate:1 1 UDP 1 %[2]s %[3]d typ host
a=end-of-candidates
`

// createServerSDP 创建服务器的SDP字符串
// 参数:
//   - addr: *net.UDPAddr UDP地址对象
//   - ufrag: string ICE用户片段
//   - fingerprint: multihash.DecodedMultihash 解码后的多重哈希指纹
//
// 返回值:
//   - string 格式化后的SDP字符串
//   - error 可能的错误
func createServerSDP(addr *net.UDPAddr, ufrag string, fingerprint multihash.DecodedMultihash) (string, error) {
	// 根据IP地址类型确定IP版本
	ipVersion := "IP4"
	if addr.IP.To4() == nil {
		ipVersion = "IP6"
	}

	// 获取支持的SDP字符串
	sdpString, err := getSupportedSDPString(fingerprint.Code)
	if err != nil {
		log.Debugf("获取支持的SDP字符串时出错: %s", err)
		return "", err
	}

	// 构建指纹字符串
	var builder strings.Builder
	builder.Grow(len(fingerprint.Digest)*3 + 8)
	builder.WriteString(sdpString)
	builder.WriteByte(' ')
	builder.WriteString(encodeInterspersedHex(fingerprint.Digest))
	fp := builder.String()

	// 使用格式化字符串生成SDP
	return fmt.Sprintf(
		serverSDP,
		ipVersion,
		addr.IP,
		addr.Port,
		ufrag,
		fp,
	), nil
}

// getSupportedSDPHash 将多重哈希代码转换为对应的加密哈希算法
// 参数:
//   - code: uint64 多重哈希代码
//
// 返回值:
//   - crypto.Hash 对应的加密哈希算法
//   - bool 是否找到对应的算法
//
// 注意:
//   - 如果找不到对应的加密哈希算法,返回 (0, false)
func getSupportedSDPHash(code uint64) (crypto.Hash, bool) {
	switch code {
	case multihash.MD5:
		return crypto.MD5, true
	case multihash.SHA1:
		return crypto.SHA1, true
	case multihash.SHA3_224:
		return crypto.SHA3_224, true
	case multihash.SHA2_256:
		return crypto.SHA256, true
	case multihash.SHA3_384:
		return crypto.SHA3_384, true
	case multihash.SHA2_512:
		return crypto.SHA512, true
	default:
		return 0, false
	}
}

// getSupportedSDPString 将多重哈希代码转换为pion识别的指纹算法字符串格式
// 参数:
//   - code: uint64 多重哈希代码
//
// 返回值:
//   - string 对应的算法字符串
//   - error 可能的错误
func getSupportedSDPString(code uint64) (string, error) {
	// 基于 (crypto.Hash).String() 的值
	switch code {
	case multihash.MD5:
		return "md5", nil
	case multihash.SHA1:
		return "sha-1", nil
	case multihash.SHA3_224:
		return "sha3-224", nil
	case multihash.SHA2_256:
		return "sha-256", nil
	case multihash.SHA3_384:
		return "sha3-384", nil
	case multihash.SHA2_512:
		return "sha-512", nil
	default:
		return "", fmt.Errorf("不支持的哈希代码 (%d)", code)
	}
}
