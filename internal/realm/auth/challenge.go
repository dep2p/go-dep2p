package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"time"
)

// ============================================================================
//                              Nonce 生成
// ============================================================================

// GenerateNonce 生成随机 nonce
func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("%w: failed to generate nonce: %v", ErrInvalidNonce, err)
	}
	return nonce, nil
}

// ============================================================================
//                              证明计算
// ============================================================================

// ComputeProof 计算认证证明
//
// 证明格式：HMAC-SHA256(AuthKey, nonce||peerID||timestamp)
func ComputeProof(authKey, nonce []byte, peerID string, timestamp int64) []byte {
	h := hmac.New(sha256.New, authKey)

	// 写入 nonce
	h.Write(nonce)

	// 写入 peerID
	h.Write([]byte(peerID))

	// 写入 timestamp
	timestampBytes := make([]byte, 8)
	timestampBytes[0] = byte(timestamp >> 56)
	timestampBytes[1] = byte(timestamp >> 48)
	timestampBytes[2] = byte(timestamp >> 40)
	timestampBytes[3] = byte(timestamp >> 32)
	timestampBytes[4] = byte(timestamp >> 24)
	timestampBytes[5] = byte(timestamp >> 16)
	timestampBytes[6] = byte(timestamp >> 8)
	timestampBytes[7] = byte(timestamp)
	h.Write(timestampBytes)

	return h.Sum(nil)
}

// ============================================================================
//                              时间戳验证
// ============================================================================

// VerifyTimestamp 验证时间戳是否在有效窗口内
//
// 防重放攻击：只接受时间窗口内的时间戳。
func VerifyTimestamp(timestamp int64, window time.Duration) bool {
	now := time.Now().Unix()
	diff := now - timestamp

	// 允许的时间差（秒）
	maxDiff := int64(window.Seconds())

	// 时间戳可以在过去或未来（考虑时钟偏移）
	if diff < -maxDiff || diff > maxDiff {
		return false
	}

	return true
}

// ============================================================================
//                              挑战-响应协议
// ============================================================================

// ChallengeHandler 挑战处理器
type ChallengeHandler struct {
	// 配置
	timeout      time.Duration
	replayWindow time.Duration
	maxRetries   int
}

// NewChallengeHandler 创建挑战处理器
func NewChallengeHandler(timeout, replayWindow time.Duration, maxRetries int) *ChallengeHandler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if replayWindow <= 0 {
		replayWindow = 5 * time.Minute
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}

	return &ChallengeHandler{
		timeout:      timeout,
		replayWindow: replayWindow,
		maxRetries:   maxRetries,
	}
}

// PerformChallenge 执行挑战-响应认证（客户端侧）
//
// 认证流程：
//  1. 发送 AuthRequest
//  2. 接收 AuthChallenge
//  3. 计算并发送 AuthResponse
//  4. 接收 AuthResult
func (h *ChallengeHandler) PerformChallenge(
	ctx context.Context,
	peerID string,
	realmID string,
	authKey []byte,
	sendRequest func([]byte) error,
	receiveChallenge func() ([]byte, error),
	sendResponse func([]byte) error,
	receiveResult func() ([]byte, error),
) error {
	// 创建超时上下文（cancel 用于确保资源释放）
	_, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	// 1. 发送认证请求
	timestamp := time.Now().Unix()
	request := &ChallengeRequest{
		PeerID:    peerID,
		RealmID:   realmID,
		Timestamp: timestamp,
	}

	requestData := h.encodeRequest(request)
	if err := sendRequest(requestData); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// 2. 接收挑战
	challengeData, err := receiveChallenge()
	if err != nil {
		return fmt.Errorf("failed to receive challenge: %w", err)
	}

	challenge, err := h.decodeChallenge(challengeData)
	if err != nil {
		return fmt.Errorf("failed to decode challenge: %w", err)
	}

	// 验证挑战时间戳
	if !VerifyTimestamp(challenge.Timestamp, h.replayWindow) {
		return ErrTimestampExpired
	}

	// 3. 计算证明
	proof := ComputeProof(authKey, challenge.Nonce, peerID, challenge.Timestamp)

	response := &ProofResponse{
		Proof:     proof,
		Timestamp: challenge.Timestamp,
	}

	responseData := h.encodeResponse(response)
	if err := sendResponse(responseData); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}

	// 4. 接收结果
	resultData, err := receiveResult()
	if err != nil {
		return fmt.Errorf("failed to receive result: %w", err)
	}

	result, err := h.decodeResult(resultData)
	if err != nil {
		return fmt.Errorf("failed to decode result: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("%w: %s", ErrAuthFailed, result.Error)
	}

	return nil
}

// HandleChallenge 处理挑战请求（服务端侧）
//
// 认证流程：
//  1. 接收 AuthRequest
//  2. 生成并发送 AuthChallenge
//  3. 接收 AuthResponse
//  4. 验证并发送 AuthResult
//
// 返回值：
//   - peerID: 认证请求方的 PeerID（从请求消息中解析）
//   - error: 认证错误
//
// 返回实际的 PeerID，而不是依赖 stream.Conn().RemotePeer()
// 中继连接时 stream.Conn().RemotePeer() 返回的是 Relay 节点，不是真正的对端
func (h *ChallengeHandler) HandleChallenge(
	ctx context.Context,
	authKey []byte,
	receiveRequest func() ([]byte, error),
	sendChallenge func([]byte) error,
	receiveResponse func() ([]byte, error),
	sendResult func([]byte) error,
) (peerID string, err error) {
	// 创建超时上下文（cancel 用于确保资源释放）
	_, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	// 1. 接收认证请求
	requestData, err := receiveRequest()
	if err != nil {
		return "", fmt.Errorf("failed to receive request: %w", err)
	}

	request, err := h.decodeRequest(requestData)
	if err != nil {
		return "", fmt.Errorf("failed to decode request: %w", err)
	}

	// 从请求消息中获取对方的 PeerID
	// 中继连接时 stream.Conn().RemotePeer() 返回 Relay ID，这里返回实际的对端 ID
	peerID = request.PeerID

	// 验证请求时间戳
	if !VerifyTimestamp(request.Timestamp, h.replayWindow) {
		// 发送失败结果
		failResult := &AuthenticationResult{
			Success: false,
			Error:   "timestamp expired",
		}
		sendResult(h.encodeResult(failResult))
		return peerID, ErrTimestampExpired
	}

	// 2. 生成挑战
	nonce, err := GenerateNonce()
	if err != nil {
		return peerID, fmt.Errorf("failed to generate nonce: %w", err)
	}

	challengeTimestamp := time.Now().Unix()
	challenge := &ChallengeResponse{
		Nonce:     nonce,
		Timestamp: challengeTimestamp,
	}

	challengeData := h.encodeChallenge(challenge)
	if err := sendChallenge(challengeData); err != nil {
		return peerID, fmt.Errorf("failed to send challenge: %w", err)
	}

	// 3. 接收响应
	responseData, err := receiveResponse()
	if err != nil {
		return peerID, fmt.Errorf("failed to receive response: %w", err)
	}

	response, err := h.decodeResponse(responseData)
	if err != nil {
		return peerID, fmt.Errorf("failed to decode response: %w", err)
	}

	// 4. 验证证明
	expectedProof := ComputeProof(authKey, nonce, peerID, challengeTimestamp)

	if !hmac.Equal(response.Proof, expectedProof) {
		// 发送失败结果
		failResult := &AuthenticationResult{
			Success: false,
			Error:   "invalid proof",
		}
		sendResult(h.encodeResult(failResult))
		return peerID, ErrAuthFailed
	}

	// 发送成功结果
	successResult := &AuthenticationResult{
		Success: true,
		Error:   "",
	}
	if err := sendResult(h.encodeResult(successResult)); err != nil {
		return peerID, fmt.Errorf("failed to send result: %w", err)
	}

	return peerID, nil
}

// ============================================================================
//                              消息编解码（简化实现）
// ============================================================================

// ChallengeRequest 挑战请求
type ChallengeRequest struct {
	PeerID    string
	RealmID   string
	Timestamp int64
}

// ChallengeResponse 挑战响应
type ChallengeResponse struct {
	Nonce     []byte
	Timestamp int64
}

// ProofResponse 证明响应
type ProofResponse struct {
	Proof     []byte
	Timestamp int64
}

// AuthenticationResult 认证结果
type AuthenticationResult struct {
	Success bool
	Error   string
}

// 协议版本和魔数常量
const (
	// ChallengeProtocolVersion 挑战协议版本
	ChallengeProtocolVersion uint8 = 1

	// ChallengeMagicHigh 魔数高字节
	ChallengeMagicHigh byte = 0xCA
	// ChallengeMagicLow 魔数低字节
	ChallengeMagicLow byte = 0x01

	// ChallengeMagic 魔数标识（用于比较）
	ChallengeMagic uint16 = 0xCA01
)

// encodeRequest 编码请求
//
// Phase 11 修复：使用标准化格式，包含版本号和魔数
// 格式：[magic(2)][version(1)][peerID length(2)][peerID][realmID length(2)][realmID][timestamp(8)]
func (h *ChallengeHandler) encodeRequest(req *ChallengeRequest) []byte {
	result := make([]byte, 0, 256)

	// 魔数（2 字节）
	result = append(result, ChallengeMagicHigh, ChallengeMagicLow)

	// 版本号（1 字节）
	result = append(result, ChallengeProtocolVersion)

	// PeerID（2 字节长度 + 数据）
	peerIDBytes := []byte(req.PeerID)
	result = append(result, byte(len(peerIDBytes)>>8), byte(len(peerIDBytes)))
	result = append(result, peerIDBytes...)

	// RealmID（2 字节长度 + 数据）
	realmIDBytes := []byte(req.RealmID)
	result = append(result, byte(len(realmIDBytes)>>8), byte(len(realmIDBytes)))
	result = append(result, realmIDBytes...)

	// Timestamp（8 字节）
	result = appendInt64(result, req.Timestamp)

	return result
}

// decodeRequest 解码请求
//
// Phase 11 修复：支持新旧格式兼容
func (h *ChallengeHandler) decodeRequest(data []byte) (*ChallengeRequest, error) {
	if len(data) < 3 {
		return nil, ErrInvalidProof
	}

	offset := 0

	// 检查是否为新格式（魔数）
	magic := uint16(data[0])<<8 | uint16(data[1])
	if magic == ChallengeMagic {
		// 新格式
		offset = 2

		// 版本号
		version := data[offset]
		offset++
		if version > ChallengeProtocolVersion {
			return nil, fmt.Errorf("unsupported protocol version: %d", version)
		}

		// PeerID（2 字节长度）
		if offset+2 > len(data) {
			return nil, ErrInvalidProof
		}
		peerIDLen := int(data[offset])<<8 | int(data[offset+1])
		offset += 2
		if offset+peerIDLen > len(data) {
			return nil, ErrInvalidProof
		}
		peerID := string(data[offset : offset+peerIDLen])
		offset += peerIDLen

		// RealmID（2 字节长度）
		if offset+2 > len(data) {
			return nil, ErrInvalidProof
		}
		realmIDLen := int(data[offset])<<8 | int(data[offset+1])
		offset += 2
		if offset+realmIDLen > len(data) {
			return nil, ErrInvalidProof
		}
		realmID := string(data[offset : offset+realmIDLen])
		offset += realmIDLen

		// Timestamp
		if offset+8 > len(data) {
			return nil, ErrInvalidProof
		}
		timestamp := parseInt64(data[offset : offset+8])

		return &ChallengeRequest{
			PeerID:    peerID,
			RealmID:   realmID,
			Timestamp: timestamp,
		}, nil
	}

	// 旧格式兼容（无魔数）
	// PeerID（1 字节长度）
	peerIDLen := int(data[offset])
	offset++
	if offset+peerIDLen > len(data) {
		return nil, ErrInvalidProof
	}
	peerID := string(data[offset : offset+peerIDLen])
	offset += peerIDLen

	// RealmID（1 字节长度）
	if offset >= len(data) {
		return nil, ErrInvalidProof
	}
	realmIDLen := int(data[offset])
	offset++
	if offset+realmIDLen > len(data) {
		return nil, ErrInvalidProof
	}
	realmID := string(data[offset : offset+realmIDLen])
	offset += realmIDLen

	// Timestamp
	if offset+8 > len(data) {
		return nil, ErrInvalidProof
	}
	timestamp := parseInt64(data[offset : offset+8])

	return &ChallengeRequest{
		PeerID:    peerID,
		RealmID:   realmID,
		Timestamp: timestamp,
	}, nil
}

// encodeChallenge 编码挑战
//
// Phase 11 修复：使用标准化格式
// 格式：[magic(2)][version(1)][nonce(32)][timestamp(8)]
func (h *ChallengeHandler) encodeChallenge(chal *ChallengeResponse) []byte {
	result := make([]byte, 0, 43)

	// 魔数（2 字节）
	result = append(result, ChallengeMagicHigh, ChallengeMagicLow)

	// 版本号（1 字节）
	result = append(result, ChallengeProtocolVersion)

	// Nonce（32 字节）
	result = append(result, chal.Nonce...)

	// Timestamp（8 字节）
	result = appendInt64(result, chal.Timestamp)

	return result
}

// decodeChallenge 解码挑战
//
// Phase 11 修复：支持新旧格式兼容
func (h *ChallengeHandler) decodeChallenge(data []byte) (*ChallengeResponse, error) {
	if len(data) < 40 {
		return nil, ErrInvalidProof
	}

	// 检查是否为新格式（魔数）
	magic := uint16(data[0])<<8 | uint16(data[1])
	if magic == ChallengeMagic && len(data) >= 43 {
		// 新格式
		version := data[2]
		if version > ChallengeProtocolVersion {
			return nil, fmt.Errorf("unsupported protocol version: %d", version)
		}

		return &ChallengeResponse{
			Nonce:     data[3:35],
			Timestamp: parseInt64(data[35:43]),
		}, nil
	}

	// 旧格式
	return &ChallengeResponse{
		Nonce:     data[:32],
		Timestamp: parseInt64(data[32:40]),
	}, nil
}

// encodeResponse 编码响应
//
// Phase 11 修复：使用标准化格式
// 格式：[magic(2)][version(1)][proof_length(2)][proof][timestamp(8)]
func (h *ChallengeHandler) encodeResponse(resp *ProofResponse) []byte {
	result := make([]byte, 0, 48)

	// 魔数（2 字节）
	result = append(result, ChallengeMagicHigh, ChallengeMagicLow)

	// 版本号（1 字节）
	result = append(result, ChallengeProtocolVersion)

	// Proof 长度（2 字节）
	proofLen := len(resp.Proof)
	result = append(result, byte(proofLen>>8), byte(proofLen))

	// Proof
	result = append(result, resp.Proof...)

	// Timestamp（8 字节）
	result = appendInt64(result, resp.Timestamp)

	return result
}

// decodeResponse 解码响应
//
// Phase 11 修复：支持新旧格式兼容
func (h *ChallengeHandler) decodeResponse(data []byte) (*ProofResponse, error) {
	if len(data) < 40 {
		return nil, ErrInvalidProof
	}

	// 检查是否为新格式（魔数）
	magic := uint16(data[0])<<8 | uint16(data[1])
	if magic == ChallengeMagic && len(data) >= 13 {
		// 新格式
		version := data[2]
		if version > ChallengeProtocolVersion {
			return nil, fmt.Errorf("unsupported protocol version: %d", version)
		}

		// Proof 长度
		proofLen := int(data[3])<<8 | int(data[4])
		if len(data) < 5+proofLen+8 {
			return nil, ErrInvalidProof
		}

		return &ProofResponse{
			Proof:     data[5 : 5+proofLen],
			Timestamp: parseInt64(data[5+proofLen : 5+proofLen+8]),
		}, nil
	}

	// 旧格式（假设 32 字节 proof）
	return &ProofResponse{
		Proof:     data[:32],
		Timestamp: parseInt64(data[32:40]),
	}, nil
}

// encodeResult 编码结果
func (h *ChallengeHandler) encodeResult(result *AuthenticationResult) []byte {
	data := make([]byte, 0, 256)

	// Success (1 byte)
	if result.Success {
		data = append(data, 1)
	} else {
		data = append(data, 0)
	}

	// Error
	data = append(data, byte(len(result.Error)))
	data = append(data, []byte(result.Error)...)

	return data
}

// decodeResult 解码结果
func (h *ChallengeHandler) decodeResult(data []byte) (*AuthenticationResult, error) {
	if len(data) < 2 {
		return nil, ErrInvalidProof
	}

	result := &AuthenticationResult{
		Success: data[0] == 1,
	}

	errorLen := int(data[1])
	if errorLen > 0 {
		if len(data) < 2+errorLen {
			return nil, ErrInvalidProof
		}
		result.Error = string(data[2 : 2+errorLen])
	}

	return result, nil
}
