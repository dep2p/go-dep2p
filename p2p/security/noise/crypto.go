package noise

import (
	"errors"
)

// encrypt 调用密码器的加密功能
// 参数:
//   - out: []byte 输出缓冲区,用于存储加密后的密文
//   - plaintext: []byte 待加密的明文数据
//
// 返回值:
//   - []byte 加密后的密文数据,包含认证标签
//   - error 加密过程中的错误
//
// 注意:
//   - 通常应传入一个长度为0但容量足够的切片作为out参数,以避免内存分配
//   - 返回的切片长度为密文长度(包含16字节认证标签)
//   - 如果out切片容量足够,则不会发生内存分配
//   - noise-dep2p使用poly1305 MAC函数,会增加16字节认证标签开销
func (s *secureSession) encrypt(out, plaintext []byte) ([]byte, error) {
	// 检查加密器是否已初始化
	if s.enc == nil {
		log.Debugf("无法加密,握手未完成")
		return nil, errors.New("无法加密,握手未完成")
	}
	// 调用加密器进行加密
	return s.enc.Encrypt(out, nil, plaintext)
}

// decrypt 调用密码器的解密功能
// 参数:
//   - out: []byte 输出缓冲区,用于存储解密后的明文
//   - ciphertext: []byte 待解密的密文数据
//
// 返回值:
//   - []byte 解密后的明文数据,不包含认证标签
//   - error 解密过程中的错误
//
// 注意:
//   - 通常应传入一个长度为0但容量足够的切片作为out参数,以避免内存分配
//   - 返回的切片长度为明文长度(不包含认证标签)
//   - 如果out切片容量足够,则不会发生内存分配
func (s *secureSession) decrypt(out, ciphertext []byte) ([]byte, error) {
	// 检查解密器是否已初始化
	if s.dec == nil {
		log.Debugf("无法解密,握手未完成")
		return nil, errors.New("无法解密,握手未完成")
	}
	// 调用解密器进行解密
	return s.dec.Decrypt(out, nil, ciphertext)
}
