package handshake

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/core/crypto"

	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
)

// PeerIDAuthScheme 定义了对等节点认证方案的名称
const PeerIDAuthScheme = "dep2p-PeerID"

// challengeLen 定义了挑战数据的长度
const challengeLen = 32

// maxHeaderSize 定义了头部的最大大小
const maxHeaderSize = 2048

// peerIDAuthSchemeBytes 是 PeerIDAuthScheme 的字节表示
var peerIDAuthSchemeBytes = []byte(PeerIDAuthScheme)

// errTooBig 表示头部值太大的错误
var errTooBig = errors.New("头部值太大")

// errInvalid 表示头部值无效的错误
var errInvalid = errors.New("无效的头部值")

// errNotRan 表示未运行的错误
var errNotRan = errors.New("未运行。请先调用 Run()")

// randReader 是随机数读取器,可在测试中修改
var randReader = rand.Reader

// nowFn 是获取当前时间的函数,可在测试中修改
var nowFn = time.Now

// params 表示通过头部传递的参数
// 所有字段都使用 []byte 以避免内存分配
type params struct {
	bearerTokenB64  []byte // bearer token 的 base64 编码
	challengeClient []byte // 客户端挑战数据
	challengeServer []byte // 服务端挑战数据
	opaqueB64       []byte // 不透明数据的 base64 编码
	publicKeyB64    []byte // 公钥的 base64 编码
	sigB64          []byte // 签名的 base64 编码
}

// parsePeerIDAuthSchemeParams 解析头部字符串中的 PeerID 认证方案参数
// 零内存分配
// 参数:
//   - headerVal: 头部值字节切片
//
// 返回:
//   - error: 解析错误
func (p *params) parsePeerIDAuthSchemeParams(headerVal []byte) error {
	// 检查头部大小是否超过限制
	if len(headerVal) > maxHeaderSize {
		return errTooBig
	}

	// 查找认证方案名称的起始位置
	startIdx := bytes.Index(headerVal, peerIDAuthSchemeBytes)
	if startIdx == -1 {
		return nil
	}

	// 跳过方案名称
	headerVal = headerVal[startIdx+len(PeerIDAuthScheme):]

	// 解析参数
	advance, token, err := splitAuthHeaderParams(headerVal, true)
	for ; err == nil; advance, token, err = splitAuthHeaderParams(headerVal, true) {
		headerVal = headerVal[advance:]
		bs := token
		splitAt := bytes.Index(bs, []byte("="))
		if splitAt == -1 {
			return errInvalid
		}
		kB := bs[:splitAt]
		v := bs[splitAt+1:]
		if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
			return errInvalid
		}
		v = v[1 : len(v)-1] // 去除引号

		// 根据参数名称设置对应的值
		switch string(kB) {
		case "bearer":
			p.bearerTokenB64 = v
		case "challenge-client":
			p.challengeClient = v
		case "challenge-server":
			p.challengeServer = v
		case "opaque":
			p.opaqueB64 = v
		case "public-key":
			p.publicKeyB64 = v
		case "sig":
			p.sigB64 = v
		}
	}
	if err == bufio.ErrFinalToken {
		err = nil
	}
	return err
}

// splitAuthHeaderParams 分割认证头部参数
// 参数:
//   - data: 要分割的数据
//   - atEOF: 是否到达文件末尾
//
// 返回:
//   - advance: 前进的字节数
//   - token: 分割出的令牌
//   - err: 分割错误
func splitAuthHeaderParams(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if len(data) == 0 && atEOF {
		log.Debugf("数据为空且到达文件末尾")
		return 0, nil, bufio.ErrFinalToken
	}

	// 跳过前导空格和逗号
	start := 0
	for start < len(data) && (data[start] == ' ' || data[start] == ',') {
		start++
	}
	if start == len(data) {
		return len(data), nil, nil
	}

	// 查找参数结束位置
	end := start + 1
	for end < len(data) && data[end] != ' ' && data[end] != ',' {
		end++
	}
	token = data[start:end]

	// 检查是否为参数
	if !bytes.ContainsAny(token, "=") {
		// 不是参数,可能是下一个方案,结束解析
		return len(data), nil, bufio.ErrFinalToken
	}

	return end, token, nil
}

// headerBuilder 用于构建头部字符串
type headerBuilder struct {
	b              strings.Builder // 字符串构建器
	pastFirstField bool            // 是否已经写入第一个字段
}

// clear 清空构建器
func (h *headerBuilder) clear() {
	h.b.Reset()
	h.pastFirstField = false
}

// writeScheme 写入认证方案
// 参数:
//   - scheme: 认证方案名称
func (h *headerBuilder) writeScheme(scheme string) {
	h.b.WriteString(scheme)
	h.b.WriteByte(' ')
}

// maybeAddComma 在需要时添加逗号
func (h *headerBuilder) maybeAddComma() {
	if !h.pastFirstField {
		h.pastFirstField = true
		return
	}
	h.b.WriteString(", ")
}

// writeParamB64 写入 base64 编码的参数
// 参数:
//   - buf: 缓冲区
//   - key: 参数名
//   - val: 参数值
func (h *headerBuilder) writeParamB64(buf []byte, key string, val []byte) {
	if buf == nil {
		buf = make([]byte, base64.URLEncoding.EncodedLen(len(val)))
	}
	encodedVal := base64.URLEncoding.AppendEncode(buf[:0], val)
	h.writeParam(key, encodedVal)
}

// writeParam 写入参数
// 参数:
//   - key: 参数名
//   - val: 参数值
func (h *headerBuilder) writeParam(key string, val []byte) {
	if len(val) == 0 {
		return
	}
	h.maybeAddComma()

	h.b.Grow(len(key) + len(`="`) + len(val) + 1)
	h.b.WriteString(key)
	h.b.WriteString(`="`)
	h.b.Write(val)
	h.b.WriteByte('"')
}

// sigParam 表示签名参数
type sigParam struct {
	k string // 参数名
	v []byte // 参数值
}

// verifySig 验证签名
// 参数:
//   - publicKey: 公钥
//   - prefix: 前缀
//   - signedParts: 已签名的部分
//   - sig: 签名数据
//
// 返回:
//   - error: 验证错误
func verifySig(publicKey crypto.PubKey, prefix string, signedParts []sigParam, sig []byte) error {
	if publicKey == nil {
		log.Debugf("没有用于验证签名的公钥")
		return fmt.Errorf("没有用于验证签名的公钥")
	}

	b := pool.Get(4096)
	defer pool.Put(b)
	buf, err := genDataToSign(b[:0], prefix, signedParts)
	if err != nil {
		log.Debugf("生成签名数据失败: %v", err)
		return err
	}
	ok, err := publicKey.Verify(buf, sig)
	if err != nil {
		log.Debugf("验证签名失败: %v", err)
		return err
	}
	if !ok {
		log.Debugf("签名验证失败")
		return fmt.Errorf("签名验证失败")
	}

	return nil
}

// sign 生成签名
// 参数:
//   - privKey: 私钥
//   - prefix: 前缀
//   - partsToSign: 要签名的部分
//
// 返回:
//   - []byte: 签名数据
//   - error: 签名错误
func sign(privKey crypto.PrivKey, prefix string, partsToSign []sigParam) ([]byte, error) {
	if privKey == nil {
		log.Debugf("没有可用于签名的私钥")
		return nil, fmt.Errorf("没有可用于签名的私钥")
	}
	b := pool.Get(4096)
	defer pool.Put(b)
	buf, err := genDataToSign(b[:0], prefix, partsToSign)
	if err != nil {
		log.Debugf("生成待签名数据失败: %v", err)
		return nil, err
	}
	return privKey.Sign(buf)
}

// genDataToSign 生成待签名数据
// 参数:
//   - buf: 缓冲区
//   - prefix: 前缀
//   - parts: 要签名的部分
//
// 返回:
//   - []byte: 生成的数据
//   - error: 生成错误
func genDataToSign(buf []byte, prefix string, parts []sigParam) ([]byte, error) {
	// 按字典序排序
	slices.SortFunc(parts, func(a, b sigParam) int {
		return strings.Compare(a.k, b.k)
	})

	// 添加前缀
	buf = append(buf, prefix...)

	// 添加各个部分
	for _, p := range parts {
		buf = binary.AppendUvarint(buf, uint64(len(p.k)+1+len(p.v))) // +1 表示 '='
		buf = append(buf, p.k...)
		buf = append(buf, '=')
		buf = append(buf, p.v...)
	}
	return buf, nil
}
