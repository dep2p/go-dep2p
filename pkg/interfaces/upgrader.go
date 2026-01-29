// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Upgrader 接口，抽象连接升级器。
package interfaces

import (
	"context"
	"net"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// Upgrader 连接升级器接口
//
// Upgrader 负责将原始网络连接升级为安全、多路复用的 P2P 连接。
// 升级过程包括：
//  1. 安全协议协商（multistream-select）
//  2. 安全握手（TLS/Noise）
//  3. 多路复用协商（multistream-select）
//  4. 多路复用设置（yamux）
//
// QUIC 连接因自带加密和多路复用，会跳过升级流程。
type Upgrader interface {
	// Upgrade 升级连接
	//
	// 参数：
	//  - ctx: 上下文（用于超时控制）
	//  - conn: 原始网络连接
	//  - dir: 连接方向（Inbound/Outbound）
	//  - remotePeer: 远程节点 ID（Outbound 必须提供，Inbound 可为空）
	//
	// 返回：
	//  - UpgradedConn: 升级后的连接
	//  - error: 错误（协商失败、握手失败等）
	//
	// 错误场景：
	//  - 协商超时
	//  - 不支持的协议
	//  - 握手失败
	//  - PeerID 不匹配
	Upgrade(ctx context.Context, conn net.Conn, dir Direction, 
	        remotePeer types.PeerID) (UpgradedConn, error)
}

// UpgradedConn 升级后的连接接口
//
// UpgradedConn 表示经过安全握手和多路复用设置后的连接。
// 它嵌入 MuxedConn 接口，并提供安全信息访问。
type UpgradedConn interface {
	MuxedConn  // 多路复用连接

	// LocalPeer 返回本地节点 ID
	LocalPeer() types.PeerID

	// RemotePeer 返回远端节点 ID
	RemotePeer() types.PeerID

	// Security 返回协商的安全协议
	// 例如："/tls/1.0.0", "/noise"
	Security() types.ProtocolID

	// Muxer 返回协商的多路复用器
	// 例如："/yamux/1.0.0"
	Muxer() string
}

// UpgraderConfig 升级器配置
type UpgraderConfig struct {
	// SecurityTransports 安全传输列表（按优先级排序）
	// 客户端会按顺序提议，服务器从中选择
	SecurityTransports []SecureTransport

	// StreamMuxers 流多路复用器列表（按优先级排序）
	// 客户端会按顺序提议，服务器从中选择
	StreamMuxers []StreamMuxer

	// ResourceManager 资源管理器（可选）
	// 用于连接资源限制
	ResourceManager ResourceManager

	// NegotiateTimeout 协议协商超时时间（默认 60s）
	NegotiateTimeout int64

	// HandshakeTimeout 握手超时时间（默认 30s）
	HandshakeTimeout int64
}
