package peer

import (
	"fmt"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/internal/catch"
	"github.com/dep2p/go-dep2p/core/peer/pb"
	"github.com/dep2p/go-dep2p/core/record"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"

	"google.golang.org/protobuf/proto"
)

// 确保 PeerRecord 实现了 record.Record 接口
var _ record.Record = (*PeerRecord)(nil)

// init 初始化函数，注册 PeerRecord 类型
func init() {
	record.RegisterType(&PeerRecord{})
}

// PeerRecordEnvelopeDomain 是用于包含在 Envelope 中的 peer records 的域字符串
const PeerRecordEnvelopeDomain = "dep2p-peer-record"

// PeerRecordEnvelopePayloadType 是用于在 Envelope 中标识 peer records 的类型提示
// 在 https://github.com/multiformats/multicodec/blob/master/table.csv 中定义，名称为 "dep2p-peer-record"
var PeerRecordEnvelopePayloadType = []byte{0x03, 0x01}

// PeerRecord 包含与其他对等节点共享的广泛有用的信息
// 目前，PeerRecord 包含对等节点的公共监听地址，未来预计会包含其他信息
type PeerRecord struct {
	// PeerID 是此记录所属对等节点的 ID
	PeerID ID

	// Addrs 包含此记录所属对等节点的公共地址
	Addrs []ma.Multiaddr

	// Seq 是一个单调递增的序列计数器，用于对 PeerRecords 进行时间排序
	// Seq 值之间的间隔未指定，但对于同一对等节点，较新的 PeerRecords 必须具有比较旧记录更大的 Seq 值
	Seq uint64
}

// NewPeerRecord 创建一个具有基于时间戳的序列号的新 PeerRecord
// 返回值：
//   - *PeerRecord: 返回的记录为空，应由调用者填充
func NewPeerRecord() *PeerRecord {
	return &PeerRecord{Seq: TimestampSeq()}
}

// PeerRecordFromAddrInfo 从 AddrInfo 结构创建 PeerRecord
// 参数：
//   - info: AddrInfo 包含对等节点的地址信息
//
// 返回值：
//   - *PeerRecord: 返回的记录将具有基于时间戳的序列号
func PeerRecordFromAddrInfo(info AddrInfo) *PeerRecord {
	rec := NewPeerRecord()
	rec.PeerID = info.ID
	rec.Addrs = info.Addrs
	return rec
}

// PeerRecordFromProtobuf 从 protobuf PeerRecord 结构创建 PeerRecord
// 参数：
//   - msg: *pb.PeerRecord protobuf 消息
//
// 返回值：
//   - *PeerRecord: 创建的 PeerRecord 实例
//   - error: 如果发生错误，返回错误信息
func PeerRecordFromProtobuf(msg *pb.PeerRecord) (*PeerRecord, error) {
	record := &PeerRecord{}

	var id ID
	if err := id.UnmarshalBinary(msg.PeerId); err != nil {
		log.Debugf("反序列化失败: %v", err)
		return nil, err
	}

	record.PeerID = id
	record.Addrs = addrsFromProtobuf(msg.Addresses)
	record.Seq = msg.Seq

	return record, nil
}

var (
	// lastTimestampMu 用于保护 lastTimestamp 的互斥锁
	lastTimestampMu sync.Mutex
	// lastTimestamp 存储最后一次生成的时间戳
	lastTimestamp uint64
)

// TimestampSeq 是一个帮助函数，用于为 PeerRecord 生成基于时间戳的序列号
// 返回值：
//   - uint64: 返回生成的序列号
func TimestampSeq() uint64 {
	now := uint64(time.Now().UnixNano())
	lastTimestampMu.Lock()
	defer lastTimestampMu.Unlock()
	// 不是所有时钟都是严格递增的，但我们需要这些序列号严格递增
	if now <= lastTimestamp {
		now = lastTimestamp + 1
	}
	lastTimestamp = now
	return now
}

// Domain 用于签名和验证包含在 Envelopes 中的 PeerRecords
// 返回值：
//   - string: 对所有 PeerRecord 实例都是常量
func (r *PeerRecord) Domain() string {
	return PeerRecordEnvelopeDomain
}

// Codec 是 PeerRecord 类型的二进制标识符
// 返回值：
//   - []byte: 对所有 PeerRecord 实例都是常量
func (r *PeerRecord) Codec() []byte {
	return PeerRecordEnvelopePayloadType
}

// UnmarshalRecord 从字节切片解析 PeerRecord
// 参数：
//   - bytes: []byte 要解析的字节数据
//
// 返回值：
//   - error: 如果发生错误，返回错误信息
func (r *PeerRecord) UnmarshalRecord(bytes []byte) (err error) {
	if r == nil {
		log.Debugf("无法反序列化PeerRecord到空接收器")
		return fmt.Errorf("无法反序列化PeerRecord到空接收器")
	}

	defer func() { catch.HandlePanic(recover(), &err, "dep2p peer record unmarshal") }()

	var msg pb.PeerRecord
	err = proto.Unmarshal(bytes, &msg)
	if err != nil {
		log.Debugf("反序列化失败: %v", err)
		return err
	}

	rPtr, err := PeerRecordFromProtobuf(&msg)
	if err != nil {
		log.Debugf("反序列化失败: %v", err)
		return err
	}
	*r = *rPtr

	return nil
}

// MarshalRecord 将 PeerRecord 序列化为字节切片
// 返回值：
//   - []byte: 序列化后的数据
//   - error: 如果发生错误，返回错误信息
func (r *PeerRecord) MarshalRecord() (res []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "dep2p peer record marshal") }()

	msg, err := r.ToProtobuf()
	if err != nil {
		log.Debugf("序列化失败: %v", err)
		return nil, err
	}
	return proto.Marshal(msg)
}

// Equal 判断另一个 PeerRecord 是否与此记录相同
// 参数：
//   - other: *PeerRecord 要比较的另一个记录
//
// 返回值：
//   - bool: 如果记录相同返回 true，否则返回 false
func (r *PeerRecord) Equal(other *PeerRecord) bool {
	if other == nil {
		return r == nil
	}
	if r.PeerID != other.PeerID {
		return false
	}
	if r.Seq != other.Seq {
		return false
	}
	if len(r.Addrs) != len(other.Addrs) {
		return false
	}
	for i := range r.Addrs {
		if !r.Addrs[i].Equal(other.Addrs[i]) {
			return false
		}
	}
	return true
}

// ToProtobuf 将 PeerRecord 转换为等效的 Protocol Buffer 结构对象
// 返回值：
//   - *pb.PeerRecord: 转换后的 protobuf 对象
//   - error: 如果发生错误，返回错误信息
func (r *PeerRecord) ToProtobuf() (*pb.PeerRecord, error) {
	idBytes, err := r.PeerID.MarshalBinary()
	if err != nil {
		log.Debugf("序列化失败: %v", err)
		return nil, err
	}
	return &pb.PeerRecord{
		PeerId:    idBytes,
		Addresses: addrsToProtobuf(r.Addrs),
		Seq:       r.Seq,
	}, nil
}

// addrsFromProtobuf 将 protobuf 地址信息转换为 Multiaddr 切片
// 参数：
//   - addrs: []*pb.PeerRecord_AddressInfo protobuf 地址信息数组
//
// 返回值：
//   - []ma.Multiaddr: 转换后的 Multiaddr 切片
func addrsFromProtobuf(addrs []*pb.PeerRecord_AddressInfo) []ma.Multiaddr {
	out := make([]ma.Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		a, err := ma.NewMultiaddrBytes(addr.Multiaddr)
		if err != nil {
			log.Debugf("创建多地址失败: %v", err)
			continue
		}
		out = append(out, a)
	}
	return out
}

// addrsToProtobuf 将 Multiaddr 切片转换为 protobuf 地址信息
// 参数：
//   - addrs: []ma.Multiaddr Multiaddr 地址切片
//
// 返回值：
//   - []*pb.PeerRecord_AddressInfo: 转换后的 protobuf 地址信息数组
func addrsToProtobuf(addrs []ma.Multiaddr) []*pb.PeerRecord_AddressInfo {
	out := make([]*pb.PeerRecord_AddressInfo, 0, len(addrs))
	for _, addr := range addrs {
		out = append(out, &pb.PeerRecord_AddressInfo{Multiaddr: addr.Bytes()})
	}
	return out
}
