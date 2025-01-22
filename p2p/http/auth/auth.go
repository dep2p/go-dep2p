package httppeeridauth

import (
	"github.com/dep2p/go-dep2p/p2p/http/auth/internal/handshake"
	logging "github.com/dep2p/log"
)

// PeerIDAuthScheme 定义了对等节点身份认证方案
// 使用 handshake 包中定义的认证方案常量
const PeerIDAuthScheme = handshake.PeerIDAuthScheme

// ProtocolID 定义了 HTTP 对等节点身份认证协议的版本标识
// 格式为: /http-peer-id-auth/版本号
const ProtocolID = "/http-peer-id-auth/1.0.0"

// log 是用于记录 HTTP 对等节点身份认证相关日志的 logger 实例
// 使用 logging 包创建一个名为 "http-peer-id-auth" 的 logger
var log = logging.Logger("http-peer-id-auth")
