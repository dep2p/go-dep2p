package pnet

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"

	logging "github.com/dep2p/log"
)

// 定义文件头和编码类型的常量
var (
	// pathPSKv1 是 V1 PSK 文件的头部标识
	pathPSKv1 = []byte("/key/swarm/psk/1.0.0/")
	// pathBin 是二进制编码的标识
	pathBin = "/bin/"
	// pathBase16 是 base16 编码的标识
	pathBase16 = "/base16/"
	// pathBase64 是 base64 编码的标识
	pathBase64 = "/base64/"
)

var log = logging.Logger("core-pnet")

// readHeader 从 bufio.Reader 中读取一行作为头部信息
// 参数:
//   - r: *bufio.Reader 用于读取数据的 Reader
//
// 返回值:
//   - []byte: 读取到的头部信息,去除了行尾的换行符
//   - error: 如果发生错误,返回错误信息
func readHeader(r *bufio.Reader) ([]byte, error) {
	// 读取一行直到遇到换行符
	header, err := r.ReadBytes('\n')
	if err != nil {
		log.Debugf("读取头部信息失败: %v", err)
		return nil, err
	}

	// 去除行尾的 \r\n
	return bytes.TrimRight(header, "\r\n"), nil
}

// expectHeader 检查读取的头部信息是否与预期相符
// 参数:
//   - r: *bufio.Reader 用于读取数据的 Reader
//   - expected: []byte 预期的头部信息
//
// 返回值:
//   - error: 如果头部不匹配或发生错误,返回错误信息
func expectHeader(r *bufio.Reader, expected []byte) error {
	// 读取头部信息
	header, err := readHeader(r)
	if err != nil {
		log.Debugf("读取头部信息失败: %v", err)
		return err
	}
	// 比较头部是否匹配
	if !bytes.Equal(header, expected) {
		log.Debugf("预期文件头为%s, 实际为%s", expected, header)
		return fmt.Errorf("预期文件头为%s, 实际为%s", expected, header)
	}
	return nil
}

// DecodeV1PSK 解码 Multicodec 编码的 V1 PSK
// 参数:
//   - in: io.Reader 包含编码 PSK 数据的输入流
//
// 返回值:
//   - PSK: 解码后的 PSK 密钥
//   - error: 如果解码过程中发生错误,返回错误信息
func DecodeV1PSK(in io.Reader) (PSK, error) {
	// 创建带缓冲的读取器
	reader := bufio.NewReader(in)
	// 检查文件头是否为 V1 PSK
	if err := expectHeader(reader, pathPSKv1); err != nil {
		log.Debugf("检查文件头失败: %v", err)
		return nil, err
	}
	// 读取编码类型头
	header, err := readHeader(reader)
	if err != nil {
		log.Debugf("读取编码类型头失败: %v", err)
		return nil, err
	}

	// 根据编码类型选择对应的解码器
	var decoder io.Reader
	switch string(header) {
	case pathBase16:
		decoder = hex.NewDecoder(reader)
	case pathBase64:
		decoder = base64.NewDecoder(base64.StdEncoding, reader)
	case pathBin:
		decoder = reader
	default:
		log.Debugf("未知编码: %s", header)
		return nil, fmt.Errorf("未知编码: %s", header)
	}
	// 读取 32 字节的 PSK
	out := make([]byte, 32)
	if _, err = io.ReadFull(decoder, out[:]); err != nil {
		log.Debugf("读取PSK失败: %v", err)
		return nil, err
	}
	return out, nil
}
