// Package auth 实现 Realm 认证机制
//
// # 模块概述
//
// auth 包提供 Realm 层的成员认证功能，支持多种认证模式。
//
// 核心职责：
//   - 预共享密钥（PSK）认证
//   - TLS 证书认证
//   - 自定义认证逻辑
//   - HKDF 密钥派生
//   - 挑战-响应协议
//   - 防重放攻击
//
// # 认证模式
//
// ## PSK 模式（推荐）
//
// 使用预共享密钥进行认证，适用于私有 Realm。
//
// 密钥派生：
//   - RealmID = HKDF(PSK, salt="dep2p-realm-id-v1", info=SHA256(PSK))
//   - AuthKey = HKDF(PSK, salt="dep2p-auth-key-v1", info=RealmID)
//
// 认证流程：
//  1. 客户端发送 AuthRequest
//  2. 服务端返回 AuthChallenge（nonce）
//  3. 客户端计算 proof = HMAC-SHA256(AuthKey, nonce||peerID||timestamp)
//  4. 服务端验证 proof
//
// ## 证书模式
//
// 使用 X.509 证书进行认证，适用于企业环境。
//
// 认证流程：
//  1. 客户端发送证书
//  2. 服务端验证证书链
//  3. 检查证书有效期和吊销状态
//  4. 返回认证结果
//
// ## 自定义模式
//
// 允许实现自定义认证逻辑。
//
// # 使用示例
//
// ## PSK 认证
//
//	// 派生 RealmID
//	psk := []byte("my-secret-key")
//	realmID := auth.DeriveRealmID(psk)
//
//	// 创建认证器
//	authenticator, err := auth.NewPSKAuthenticator(psk, "peer123")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer authenticator.Close()
//
//	// 生成证明
//	ctx := context.Background()
//	proof, err := authenticator.GenerateProof(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 验证证明
//	valid, err := authenticator.Authenticate(ctx, "peer456", proof)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if !valid {
//	    log.Println("认证失败")
//	}
//
// ## 证书认证
//
//	// 创建证书认证器
//	authenticator, err := auth.NewCertAuthenticator(
//	    "/path/to/cert.pem",
//	    "/path/to/key.pem",
//	    "peer123",
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer authenticator.Close()
//
// ## 自定义认证
//
//	// 定义验证器
//	validator := func(ctx context.Context, peerID string, proof []byte) (bool, error) {
//	    // 自定义验证逻辑
//	    return string(proof) == "secret-token", nil
//	}
//
//	// 创建认证器
//	authenticator := auth.NewCustomAuthenticator("realm123", "peer123", validator)
//
// # 安全特性
//
//  1. HKDF 密钥派生（RFC 5869）
//  2. HMAC-SHA256 消息认证
//  3. 随机 nonce（crypto/rand）
//  4. 时间戳验证（防重放攻击）
//  5. 证书链完整性验证
//
// # 配置参数
//
//   - PSK：预共享密钥（16-64 字节）
//   - Timeout：认证超时（默认 30 秒）
//   - ReplayWindow：重放窗口（默认 5 分钟）
//   - MaxRetries：最大重试次数（默认 3）
//
// # 线程安全
//
// 所有认证器实现都是线程安全的，可以并发调用。
package auth
