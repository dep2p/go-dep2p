package proto

import (
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/record"
	pbv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/pb"
	logging "github.com/dep2p/log"

	"google.golang.org/protobuf/proto"
)

var log = logging.Logger("p2p-protocol-circuitv2-voucher")

// 中继预约服务的记录域名
const RecordDomain = "dep2p-relay-rsvp"

// TODO: 在 https://github.com/multiformats/multicodec 的 multicodec 表中注册
var RecordCodec = []byte{0x03, 0x02}

// init 初始化函数,注册预约凭证类型
func init() {
	record.RegisterType(&ReservationVoucher{})
}

// ReservationVoucher 表示中继预约的凭证
type ReservationVoucher struct {
	// Relay 提供中继服务的节点ID
	Relay peer.ID
	// Peer 通过Relay接收中继服务的节点ID
	Peer peer.ID
	// Expiration 预约的过期时间
	Expiration time.Time
}

// 确保 ReservationVoucher 实现了 record.Record 接口
var _ record.Record = (*ReservationVoucher)(nil)

// Domain 返回记录的域名
// 返回值:
//   - string 记录域名
func (rv *ReservationVoucher) Domain() string {
	return RecordDomain
}

// Codec 返回记录的编解码器
// 返回值:
//   - []byte 编解码器字节数组
func (rv *ReservationVoucher) Codec() []byte {
	return RecordCodec
}

// MarshalRecord 将预约凭证序列化为字节数组
// 返回值:
//   - []byte 序列化后的字节数组
//   - error 序列化过程中的错误,如果成功则为nil
func (rv *ReservationVoucher) MarshalRecord() ([]byte, error) {
	// 将过期时间转换为Unix时间戳
	expiration := uint64(rv.Expiration.Unix())
	// 序列化为protobuf格式
	return proto.Marshal(&pbv2.ReservationVoucher{
		Relay:      []byte(rv.Relay),
		Peer:       []byte(rv.Peer),
		Expiration: &expiration,
	})
}

// UnmarshalRecord 从字节数组反序列化预约凭证
// 参数:
//   - blob: []byte 待反序列化的字节数组
//
// 返回值:
//   - error 反序列化过程中的错误,如果成功则为nil
func (rv *ReservationVoucher) UnmarshalRecord(blob []byte) error {
	// 创建protobuf消息对象
	pbrv := pbv2.ReservationVoucher{}
	// 从字节数组反序列化
	err := proto.Unmarshal(blob, &pbrv)
	if err != nil {
		log.Debugf("反序列化预约凭证失败: %v", err)
		return err
	}

	// 从字节数组解析中继节点ID
	rv.Relay, err = peer.IDFromBytes(pbrv.GetRelay())
	if err != nil {
		log.Debugf("解析中继节点ID失败: %v", err)
		return err
	}

	// 从字节数组解析对等节点ID
	rv.Peer, err = peer.IDFromBytes(pbrv.GetPeer())
	if err != nil {
		log.Debugf("解析对等节点ID失败: %v", err)
		return err
	}

	// 从Unix时间戳解析过期时间
	rv.Expiration = time.Unix(int64(pbrv.GetExpiration()), 0)
	// log.Infof("反序列化预约凭证成功: %+v", rv)
	return nil
}
