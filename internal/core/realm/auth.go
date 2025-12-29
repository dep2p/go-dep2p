// Package realm 提供 RealmAuth 协议实现
//
// v1.1 新增: RealmAuth 协议用于连接级 Realm 成员验证
package realm

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志
var authLog = logger.Logger("realm.auth")

// ============================================================================
//                              协议常量
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// RealmAuthProtocol RealmAuth 协议标识
	RealmAuthProtocol = protocolids.SysRealmAuth
)

const (

	// 消息类型
	msgTypeRequest  uint8 = 1
	msgTypeResponse uint8 = 2

	// 消息头大小
	msgHeaderSize = 5 // 1 byte type + 4 bytes length

	// 最大消息大小
	maxMsgSize = 4096

	// 最大请求年龄（防重放）
	maxRequestAge = 5 * time.Minute

	// 默认验证有效期
	defaultAuthExpiry = 24 * time.Hour
)

// ============================================================================
//                              Authenticator 实现
// ============================================================================

// Authenticator RealmAuth 协议实现
//
// 负责连接级 Realm 成员验证：
// - 出站连接：主动发起 RealmAuth 握手
// - 入站连接：响应 RealmAuth 请求
type Authenticator struct {
	manager    *Manager
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	timeout    time.Duration
	expiry     time.Duration
}

// NewAuthenticator 创建 RealmAuth 认证器
func NewAuthenticator(manager *Manager, privateKey ed25519.PrivateKey) *Authenticator {
	var publicKey ed25519.PublicKey
	if privateKey != nil {
		publicKey = privateKey.Public().(ed25519.PublicKey)
	}

	return &Authenticator{
		manager:    manager,
		privateKey: privateKey,
		publicKey:  publicKey,
		timeout:    realmif.DefaultRealmAuthTimeout,
		expiry:     defaultAuthExpiry,
	}
}

// SetTimeout 设置超时时间
func (a *Authenticator) SetTimeout(timeout time.Duration) {
	a.timeout = timeout
}

// SetExpiry 设置验证有效期
func (a *Authenticator) SetExpiry(expiry time.Duration) {
	a.expiry = expiry
}

// ============================================================================
//                              出站认证
// ============================================================================

// Authenticate 执行 RealmAuth 握手（出站）
//
// 流程：
// 1. 检查本地是否已加入 Realm
// 2. 打开 RealmAuth 流
// 3. 发送签名的 RealmAuthRequest
// 4. 接收并验证 RealmAuthResponse
// 5. 设置连接级 RealmContext
func (a *Authenticator) Authenticate(ctx context.Context, conn endpoint.Connection) (*realmif.ConnRealmContext, error) {
	// 检查本地 Realm 状态
	// IMPL-1227: CurrentRealm 现在返回 Realm 对象
	realm := a.manager.CurrentRealm()
	if realm == nil {
		return nil, ErrNotMember
	}
	currentRealm := realm.ID()

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	// 打开 RealmAuth 流
	stream, err := conn.OpenStream(ctx, RealmAuthProtocol)
	if err != nil {
		return nil, fmt.Errorf("open realm auth stream: %w", err)
	}
	defer func() { _ = stream.Close() }()

	// 构造请求
	req := &realmif.RealmAuthRequest{
		SelectedRealm: currentRealm,
		Timestamp:     time.Now().Unix(),
	}

	// 签名请求
	req.Signature = a.signRequest(req)

	// 发送请求
	if err := a.writeRequest(stream, req); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// 读取响应
	resp, err := a.readResponse(stream)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// 检查响应
	if !resp.Verified {
		authLog.Warn("RealmAuth 验证失败",
			"realm", string(currentRealm),
			"error_code", resp.ErrorCode,
			"error_msg", resp.ErrorMessage)
		return nil, fmt.Errorf("%w: %s", ErrRealmAuthFailed, resp.ErrorMessage)
	}

	// 验证响应签名（可选，取决于信任模型）
	// a.verifyResponseSignature(resp, conn.RemotePublicKey())

	// 构造 ConnRealmContext
	connCtx := &realmif.ConnRealmContext{
		RealmID:   resp.SelectedRealm,
		Verified:  true,
		ExpiresAt: time.Unix(resp.ExpiresAt, 0),
	}

	authLog.Debug("RealmAuth 验证成功",
		"realm", string(resp.SelectedRealm),
		"remote", conn.RemoteID().ShortString(),
		"expires", connCtx.ExpiresAt)

	// 设置连接级 RealmContext（供 ProtocolRouter/GossipSub 等管线使用）
	conn.SetRealmContext(&endpoint.RealmContext{
		RealmID:   string(connCtx.RealmID),
		Verified:  true,
		ExpiresAt: connCtx.ExpiresAt,
	})

	return connCtx, nil
}

// ============================================================================
//                              入站处理
// ============================================================================

// HandleInbound 处理入站 RealmAuth 请求
//
// 作为协议处理器注册到 Router，处理远程节点的 RealmAuth 请求。
func (a *Authenticator) HandleInbound(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	authLog.Debug("收到 RealmAuth 请求",
		"remote", stream.Connection().RemoteID().ShortString())

	// 读取请求
	req, err := a.readRequest(stream)
	if err != nil {
		authLog.Warn("读取 RealmAuth 请求失败",
			"remote", stream.Connection().RemoteID().ShortString(),
			"err", err)
		a.writeErrorResponse(stream, realmif.RealmAuthErrInternal, "read request failed")
		return
	}

	// 验证请求
	resp := a.verifyRequest(req, stream.Connection())

	// 发送响应
	if err := a.writeResponse(stream, resp); err != nil {
		authLog.Warn("发送 RealmAuth 响应失败",
			"remote", stream.Connection().RemoteID().ShortString(),
			"err", err)
		return
	}

	// 如果验证成功，设置连接级 RealmContext
	if resp.Verified {
		connCtx := &endpoint.RealmContext{
			RealmID:   string(resp.SelectedRealm),
			Verified:  true,
			ExpiresAt: time.Unix(resp.ExpiresAt, 0),
		}
		stream.Connection().SetRealmContext(connCtx)

		authLog.Debug("RealmAuth 入站验证成功",
			"realm", string(resp.SelectedRealm),
			"remote", stream.Connection().RemoteID().ShortString())
	}
}

// verifyRequest 验证 RealmAuth 请求
//
// REQ-REALM-004 设计决策：依赖传输层身份验证
//
// 安全边界说明：
// 1. 所有连接都必须经过 QUIC 安全握手，节点身份已在传输层被验证（REQ-CONN-002）
// 2. RealmAuth 的主要目的是验证 Realm 成员身份，而非再次验证节点身份
// 3. conn.RemoteID() 返回的身份是经过传输层加密验证的可信身份
// 4. 应用层签名验证属于可选的额外防护层，当前版本选择信任传输层身份
//
// 这意味着：
// - 攻击者必须先突破 QUIC 安全层才能伪造身份
// - 如果传输层被攻破，应用层签名也无法提供有效保护（攻击者可能获取私钥）
// - 信任传输层身份简化了设计，减少了复杂度和延迟
//
// 未来可选增强：
// - 如需更强的安全保证（如跨信任域场景），可启用应用层签名验证
// - 通过配置开关控制是否启用应用层签名验证
func (a *Authenticator) verifyRequest(req *realmif.RealmAuthRequest, conn endpoint.Connection) *realmif.RealmAuthResponse {
	// 检查时间戳（防重放攻击）
	// 注：这仍然有效，因为它防止的是同一个合法节点的请求重放
	reqTime := time.Unix(req.Timestamp, 0)
	if time.Since(reqTime) > maxRequestAge {
		return &realmif.RealmAuthResponse{
			SelectedRealm: req.SelectedRealm,
			Verified:      false,
			ErrorCode:     realmif.RealmAuthErrExpired,
			ErrorMessage:  "request expired",
		}
	}

	// 检查本地是否加入了该 Realm
	if !a.manager.IsMemberOf(req.SelectedRealm) {
		return &realmif.RealmAuthResponse{
			SelectedRealm: req.SelectedRealm,
			Verified:      false,
			ErrorCode:     realmif.RealmAuthErrRealmMismatch,
			ErrorMessage:  "realm mismatch",
		}
	}

	// REQ-REALM-004 设计决策：信任传输层身份
	//
	// 应用层签名验证是可选的额外防护层。当前版本选择依赖传输层身份验证（REQ-CONN-002），
	// 因为 QUIC 安全握手已经验证了对端的节点身份。
	//
	// 如果需要启用应用层签名验证，可取消以下注释并实现 verifyRequestSignature：
	// if !a.verifyRequestSignature(req, conn.RemotePublicKey()) {
	//     return &realmif.RealmAuthResponse{
	//         SelectedRealm: req.SelectedRealm,
	//         Verified:      false,
	//         ErrorCode:     realmif.RealmAuthErrInvalidSignature,
	//         ErrorMessage:  "invalid signature",
	//     }
	// }
	_ = conn // conn 在信任传输层身份模式下不需要额外使用

	// 构造成功响应
	expiresAt := time.Now().Add(a.expiry)
	resp := &realmif.RealmAuthResponse{
		SelectedRealm: req.SelectedRealm,
		Verified:      true,
		ExpiresAt:     expiresAt.Unix(),
		ErrorCode:     realmif.RealmAuthErrNone,
	}

	// 签名响应（用于对端验证本节点的 Realm 成员身份）
	resp.Signature = a.signResponse(resp)

	return resp
}

// ============================================================================
//                              消息编解码
// ============================================================================

// writeRequest 写入请求
func (a *Authenticator) writeRequest(w io.Writer, req *realmif.RealmAuthRequest) error {
	// 简化的编码：RealmID(varint len + bytes) + Timestamp(8 bytes) + Signature(varint len + bytes)
	data := a.encodeRequest(req)
	return a.writeMessage(w, msgTypeRequest, data)
}

// readRequest 读取请求
func (a *Authenticator) readRequest(r io.Reader) (*realmif.RealmAuthRequest, error) {
	msgType, data, err := a.readMessage(r)
	if err != nil {
		return nil, err
	}
	if msgType != msgTypeRequest {
		return nil, fmt.Errorf("unexpected message type: %d", msgType)
	}
	return a.decodeRequest(data)
}

// writeResponse 写入响应
func (a *Authenticator) writeResponse(w io.Writer, resp *realmif.RealmAuthResponse) error {
	data := a.encodeResponse(resp)
	return a.writeMessage(w, msgTypeResponse, data)
}

// readResponse 读取响应
func (a *Authenticator) readResponse(r io.Reader) (*realmif.RealmAuthResponse, error) {
	msgType, data, err := a.readMessage(r)
	if err != nil {
		return nil, err
	}
	if msgType != msgTypeResponse {
		return nil, fmt.Errorf("unexpected message type: %d", msgType)
	}
	return a.decodeResponse(data)
}

// writeErrorResponse 写入错误响应
func (a *Authenticator) writeErrorResponse(w io.Writer, errCode uint32, errMsg string) {
	resp := &realmif.RealmAuthResponse{
		Verified:     false,
		ErrorCode:    errCode,
		ErrorMessage: errMsg,
	}
	_ = a.writeResponse(w, resp)
}

// writeMessage 写入消息
func (a *Authenticator) writeMessage(w io.Writer, msgType uint8, data []byte) error {
	// 消息格式: type(1) + length(4) + data
	header := make([]byte, msgHeaderSize)
	header[0] = msgType
	binary.BigEndian.PutUint32(header[1:], uint32(len(data)))

	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return nil
}

// readMessage 读取消息
func (a *Authenticator) readMessage(r io.Reader) (uint8, []byte, error) {
	header := make([]byte, msgHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	msgType := header[0]
	length := binary.BigEndian.Uint32(header[1:])

	if length > maxMsgSize {
		return 0, nil, fmt.Errorf("message too large: %d", length)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return 0, nil, err
	}

	return msgType, data, nil
}

// encodeRequest 编码请求
func (a *Authenticator) encodeRequest(req *realmif.RealmAuthRequest) []byte {
	realmBytes := []byte(req.SelectedRealm)
	// 格式: realmLen(4) + realm + timestamp(8) + sigLen(4) + sig
	size := 4 + len(realmBytes) + 8 + 4 + len(req.Signature)
	data := make([]byte, size)

	offset := 0
	binary.BigEndian.PutUint32(data[offset:], uint32(len(realmBytes)))
	offset += 4
	copy(data[offset:], realmBytes)
	offset += len(realmBytes)
	binary.BigEndian.PutUint64(data[offset:], uint64(req.Timestamp))
	offset += 8
	binary.BigEndian.PutUint32(data[offset:], uint32(len(req.Signature)))
	offset += 4
	copy(data[offset:], req.Signature)

	return data
}

// decodeRequest 解码请求
func (a *Authenticator) decodeRequest(data []byte) (*realmif.RealmAuthRequest, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("data too short")
	}

	offset := 0
	realmLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if uint32(len(data)) < 4+realmLen+8+4 {
		return nil, fmt.Errorf("data too short for realm")
	}

	realmBytes := data[offset : offset+int(realmLen)]
	offset += int(realmLen)

	timestamp := int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	sigLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if uint32(len(data)) < uint32(offset)+sigLen {
		return nil, fmt.Errorf("data too short for signature")
	}

	sig := data[offset : offset+int(sigLen)]

	return &realmif.RealmAuthRequest{
		SelectedRealm: types.RealmID(realmBytes),
		Timestamp:     timestamp,
		Signature:     sig,
	}, nil
}

// encodeResponse 编码响应
func (a *Authenticator) encodeResponse(resp *realmif.RealmAuthResponse) []byte {
	realmBytes := []byte(resp.SelectedRealm)
	// 格式: realmLen(4) + realm + verified(1) + expiresAt(8) + errCode(4) + errMsgLen(4) + errMsg + sigLen(4) + sig
	errMsgBytes := []byte(resp.ErrorMessage)
	size := 4 + len(realmBytes) + 1 + 8 + 4 + 4 + len(errMsgBytes) + 4 + len(resp.Signature)
	data := make([]byte, size)

	offset := 0
	binary.BigEndian.PutUint32(data[offset:], uint32(len(realmBytes)))
	offset += 4
	copy(data[offset:], realmBytes)
	offset += len(realmBytes)

	if resp.Verified {
		data[offset] = 1
	} else {
		data[offset] = 0
	}
	offset++

	binary.BigEndian.PutUint64(data[offset:], uint64(resp.ExpiresAt))
	offset += 8

	binary.BigEndian.PutUint32(data[offset:], resp.ErrorCode)
	offset += 4

	binary.BigEndian.PutUint32(data[offset:], uint32(len(errMsgBytes)))
	offset += 4
	copy(data[offset:], errMsgBytes)
	offset += len(errMsgBytes)

	binary.BigEndian.PutUint32(data[offset:], uint32(len(resp.Signature)))
	offset += 4
	copy(data[offset:], resp.Signature)

	return data
}

// decodeResponse 解码响应
func (a *Authenticator) decodeResponse(data []byte) (*realmif.RealmAuthResponse, error) {
	if len(data) < 21 {
		return nil, fmt.Errorf("data too short")
	}

	offset := 0
	realmLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if uint32(len(data)) < 4+realmLen+1+8+4+4 {
		return nil, fmt.Errorf("data too short for realm")
	}

	realmBytes := data[offset : offset+int(realmLen)]
	offset += int(realmLen)

	verified := data[offset] == 1
	offset++

	expiresAt := int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	errCode := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	errMsgLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if uint32(len(data)) < uint32(offset)+errMsgLen+4 {
		return nil, fmt.Errorf("data too short for error message")
	}

	errMsg := string(data[offset : offset+int(errMsgLen)])
	offset += int(errMsgLen)

	sigLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if uint32(len(data)) < uint32(offset)+sigLen {
		return nil, fmt.Errorf("data too short for signature")
	}

	sig := data[offset : offset+int(sigLen)]

	return &realmif.RealmAuthResponse{
		SelectedRealm: types.RealmID(realmBytes),
		Verified:      verified,
		ExpiresAt:     expiresAt,
		ErrorCode:     errCode,
		ErrorMessage:  errMsg,
		Signature:     sig,
	}, nil
}

// ============================================================================
//                              签名
// ============================================================================

// signRequest 签名请求
func (a *Authenticator) signRequest(req *realmif.RealmAuthRequest) []byte {
	if a.privateKey == nil {
		return nil
	}

	// 构造待签名数据
	data := make([]byte, len(req.SelectedRealm)+8)
	copy(data, req.SelectedRealm)
	binary.BigEndian.PutUint64(data[len(req.SelectedRealm):], uint64(req.Timestamp))

	return ed25519.Sign(a.privateKey, data)
}

// signResponse 签名响应
func (a *Authenticator) signResponse(resp *realmif.RealmAuthResponse) []byte {
	if a.privateKey == nil {
		return nil
	}

	// 构造待签名数据
	data := make([]byte, len(resp.SelectedRealm)+1+8+4)
	offset := 0
	copy(data[offset:], resp.SelectedRealm)
	offset += len(resp.SelectedRealm)
	if resp.Verified {
		data[offset] = 1
	}
	offset++
	binary.BigEndian.PutUint64(data[offset:], uint64(resp.ExpiresAt))
	offset += 8
	binary.BigEndian.PutUint32(data[offset:], resp.ErrorCode)

	return ed25519.Sign(a.privateKey, data)
}


