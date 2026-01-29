// Package tls 实现 TLS 安全传输
//
// tls 使用 libp2p-tls 规范实现安全传输层，基于标准 TLS 1.3，
// 通过自签名证书携带节点公钥。
//
// # 特性
//
//   - 基于 TLS 1.3
//   - 自签名证书
//   - 公钥嵌入证书扩展
//   - 双向身份验证
//
// # 证书格式
//
// 证书包含 libp2p 扩展（OID 1.3.6.1.4.1.53594.1.1），
// 携带节点的公钥和签名。
//
// # 使用示例
//
//	transport, err := tls.NewTransport(identity)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	secureConn, err := transport.SecureOutbound(ctx, conn, remotePeer)
package tls
