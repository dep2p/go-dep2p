// Package lib 包含基础设施工具库
//
// 本目录包含与架构组件无关的通用工具库：
//
//   - crypto: 密码学原语（密钥、签名、PeerID）
//   - multiaddr: 多地址格式解析
//   - log: 日志封装
//   - protocolids: 协议 ID 常量
//   - proto: Protobuf 网络消息定义
//
// # 与 pkg/ 其他目录的关系
//
// pkg/ 目录包含三类内容：
//
//   - interfaces/: 组件公共接口（架构核心）
//   - types/: 公共类型定义（架构核心）
//   - lib/: 基础设施工具库（本目录）
//
// # 使用示例
//
//	import (
//	    "github.com/dep2p/go-dep2p/pkg/lib/crypto"
//	    "github.com/dep2p/go-dep2p/pkg/lib/log"
//	    "github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
//	)
package lib
