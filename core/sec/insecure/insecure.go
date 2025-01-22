// Package insecure 提供了一个不安全的、未加密的 SecureConn 和 SecureTransport 接口实现。
//
// 仅推荐用于测试和其他非生产环境。
package insecure

import (
	"context"
	"fmt"
	"io"
	"net"

	ci "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/sec"
	"github.com/dep2p/go-dep2p/core/sec/insecure/pb"
	logging "github.com/dep2p/log"

	"github.com/dep2p/go-dep2p/p2plib/msgio"

	"google.golang.org/protobuf/proto"
)

var log = logging.Logger("core-sec-insecure")

// ID 是在标识此安全传输时应使用的 multistream-select 协议 ID。
const ID = "/plaintext/2.0.0"

// Transport 是一个无操作的流安全传输。它不提供任何安全性，仅模拟安全方法。
// 身份方法返回本地对等节点的 ID 和私钥，以及远程对等节点提供的 ID 和公钥。
// 不对远程身份进行任何验证。
type Transport struct {
	id         peer.ID     // 本地对等节点 ID
	key        ci.PrivKey  // 本地私钥
	protocolID protocol.ID // 协议 ID
}

var _ sec.SecureTransport = &Transport{}

// NewWithIdentity 构造一个新的不安全传输。。公钥会发送给远程对等节点。
// 不提供任何安全性。
// 参数：
//   - protocolID: protocol.ID 协议标识符
//   - id: peer.ID 本地对等节点 ID
//   - key: ci.PrivKey 本地私钥
//
// 返回值：
//   - *Transport: 新创建的不安全传输实例
func NewWithIdentity(protocolID protocol.ID, id peer.ID, key ci.PrivKey) *Transport {
	return &Transport{
		protocolID: protocolID,
		id:         id,
		key:        key,
	}
}

// LocalPeer 返回传输的本地对等节点 ID。
// 返回值：
//   - peer.ID: 本地对等节点 ID
func (t *Transport) LocalPeer() peer.ID {
	return t.id
}

// SecureInbound *假装安全地*处理到给定对等节点的入站连接。
// 它发送本地对等节点的 ID 和公钥，并从远程对等节点接收相同的信息。
// 不对提供的公钥的真实性或所有权进行任何验证，且密钥交换不提供任何安全性。
// 如果远程对等节点发送的 ID 和公钥相互不一致，或者在 ID 交换期间发生网络错误， SecureInbound 可能会失败。
// 参数：
//   - ctx: context.Context 上下文对象，用于控制操作
//   - insecure: net.Conn 不安全的网络连接
//   - p: peer.ID 远程对等节点 ID
//
// 返回值：
//   - sec.SecureConn: 安全连接接口实现
//   - error: 如果发生错误，返回错误信息
func (t *Transport) SecureInbound(_ context.Context, insecure net.Conn, p peer.ID) (sec.SecureConn, error) {
	conn := &Conn{
		Conn:        insecure,
		local:       t.id,
		localPubKey: t.key.GetPublic(),
	}

	if err := conn.runHandshakeSync(); err != nil {
		log.Debugf("握手失败: %v", err)
		return nil, err
	}

	if p != "" && p != conn.remote {
		log.Debugf("远程对等节点发送了意外的对等节点 ID。预期=%s 收到=%s", p, conn.remote)
		return nil, fmt.Errorf("远程对等节点发送了意外的对等节点 ID。预期=%s 收到=%s", p, conn.remote)
	}

	return conn, nil
}

// SecureOutbound *假装安全地*处理到给定对等节点的出站连接。
// 它发送本地对等节点的 ID 和公钥，并从远程对等节点接收相同的信息。
// 不对提供的公钥的真实性或所有权进行任何验证， 且密钥交换不提供任何安全性。
// 如果远程对等节点发送的 ID 和公钥相互不一致，或者远程对等节点发送的 ID 与拨号的不匹配， SecureOutbound 可能会失败。如果在 ID 交换期间发生网络错误，也可能失败。
// 参数：
//   - ctx: context.Context 上下文对象，用于控制操作
//   - insecure: net.Conn 不安全的网络连接
//   - p: peer.ID 远程对等节点 ID
//
// 返回值：
//   - sec.SecureConn: 安全连接接口实现
//   - error: 如果发生错误，返回错误信息
func (t *Transport) SecureOutbound(_ context.Context, insecure net.Conn, p peer.ID) (sec.SecureConn, error) {
	conn := &Conn{
		Conn:        insecure,
		local:       t.id,
		localPubKey: t.key.GetPublic(),
	}

	if err := conn.runHandshakeSync(); err != nil {
		log.Debugf("握手失败: %v", err)
		return nil, err
	}

	if p != conn.remote {
		log.Debugf("远程对等节点发送了意外的对等节点 ID。预期=%s 收到=%s", p, conn.remote)
		return nil, fmt.Errorf("远程对等节点发送了意外的对等节点 ID。预期=%s 收到=%s", p, conn.remote)
	}

	return conn, nil
}

// ID 返回传输的协议 ID。
// 返回值：
//   - protocol.ID: 协议标识符
func (t *Transport) ID() protocol.ID { return t.protocolID }

// Conn 是不安全传输返回的连接类型。
type Conn struct {
	net.Conn // 底层网络连接

	local, remote             peer.ID   // 本地和远程对等节点 ID
	localPubKey, remotePubKey ci.PubKey // 本地和远程公钥
}

// makeExchangeMessage 创建用于交换的消息。
// 参数：
//   - pubkey: ci.PubKey 公钥
//
// 返回值：
//   - *pb.Exchange: 交换消息
//   - error: 如果发生错误，返回错误信息
func makeExchangeMessage(pubkey ci.PubKey) (*pb.Exchange, error) {
	keyMsg, err := ci.PublicKeyToProto(pubkey)
	if err != nil {
		log.Debugf("公钥序列化失败: %v", err)
		return nil, err
	}
	id, err := peer.IDFromPublicKey(pubkey)
	if err != nil {
		log.Debugf("从公钥创建对等节点ID失败: %v", err)
		return nil, err
	}

	return &pb.Exchange{
		Id:     []byte(id),
		Pubkey: keyMsg,
	}, nil
}

// runHandshakeSync 执行同步握手过程。
// 返回值：
//   - error: 如果发生错误，返回错误信息
func (ic *Conn) runHandshakeSync() error {
	// 如果我们在没有密钥的情况下初始化，则按照 plaintext/1.0.0 的方式处理(不做任何事)
	if ic.localPubKey == nil {
		return nil
	}

	// 生成交换消息
	msg, err := makeExchangeMessage(ic.localPubKey)
	if err != nil {
		log.Debugf("创建交换消息失败: %v", err)
		return err
	}

	// 发送我们的交换消息并读取对方的
	remoteMsg, err := readWriteMsg(ic.Conn, msg)
	if err != nil {
		log.Debugf("读取交换消息失败: %v", err)
		return err
	}

	// 从消息中提取远程 ID 和公钥
	remotePubkey, err := ci.PublicKeyFromProto(remoteMsg.Pubkey)
	if err != nil {
		log.Debugf("从公钥创建对等节点ID失败: %v", err)
		return err
	}

	remoteID, err := peer.IDFromBytes(remoteMsg.Id)
	if err != nil {
		log.Debugf("从公钥创建对等节点ID失败: %v", err)
		return err
	}

	// 验证 ID 是否与公钥匹配
	if !remoteID.MatchesPublicKey(remotePubkey) {
		calculatedID, _ := peer.IDFromPublicKey(remotePubkey)
		log.Debugf("远程对等节点 ID 与公钥不匹配。id=%s 计算得到的_id=%s", remoteID, calculatedID)
		return fmt.Errorf("远程对等节点 ID 与公钥不匹配。id=%s 计算得到的_id=%s", remoteID, calculatedID)
	}

	// 将远程 ID 和密钥添加到连接状态
	ic.remotePubKey = remotePubkey
	ic.remote = remoteID
	return nil
}

// readWriteMsg 同时读写消息。
// 参数：
//   - rw: io.ReadWriter 读写接口
//   - out: *pb.Exchange 要发送的交换消息
//
// 返回值：
//   - *pb.Exchange: 接收到的交换消息
//   - error: 如果发生错误，返回错误信息
func readWriteMsg(rw io.ReadWriter, out *pb.Exchange) (*pb.Exchange, error) {
	const maxMessageSize = 1 << 16

	outBytes, err := proto.Marshal(out)
	if err != nil {
		log.Debugf("序列化失败: %v", err)
		return nil, err
	}
	wresult := make(chan error)
	go func() {
		w := msgio.NewVarintWriter(rw)
		wresult <- w.WriteMsg(outBytes)
	}()

	r := msgio.NewVarintReaderSize(rw, maxMessageSize)
	b, err1 := r.ReadMsg()

	// 始终等待读取完成。
	err2 := <-wresult

	if err1 != nil {
		log.Debugf("读取消息失败: %v", err1)
		return nil, err1
	}
	if err2 != nil {
		r.ReleaseMsg(b)
		log.Debugf("写入消息失败: %v", err2)
		return nil, err2
	}
	inMsg := new(pb.Exchange)
	err = proto.Unmarshal(b, inMsg)
	return inMsg, err
}

// LocalPeer 返回本地对等节点 ID。
// 返回值：
//   - peer.ID: 本地对等节点 ID
func (ic *Conn) LocalPeer() peer.ID {
	return ic.local
}

// RemotePeer 返回远程对等节点 ID。
// 返回值：
//   - peer.ID: 如果是拨号发起方，返回远程对等节点 ID；否则返回空字符串
func (ic *Conn) RemotePeer() peer.ID {
	return ic.remote
}

// RemotePublicKey 返回远程对等节点的公钥。
// 返回值：
//   - ci.PubKey: 远程对等节点的公钥
func (ic *Conn) RemotePublicKey() ci.PubKey {
	return ic.remotePubKey
}

// ConnState 返回安全连接的状态信息。
// 返回值：
//   - network.ConnectionState: 连接状态信息
func (ic *Conn) ConnState() network.ConnectionState {
	return network.ConnectionState{}
}

var _ sec.SecureTransport = (*Transport)(nil)
var _ sec.SecureConn = (*Conn)(nil)
var _ sec.SecureConn = (*Conn)(nil)
