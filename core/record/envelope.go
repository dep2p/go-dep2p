package record

import (
	"bytes"
	"errors"
	"sync"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/internal/catch"
	"github.com/dep2p/go-dep2p/core/record/pb"

	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"

	"github.com/dep2p/go-dep2p/multiformats/varint"
	"google.golang.org/protobuf/proto"
)

// Envelope 包含一个任意的 []byte 负载，由 dep2p 节点签名
// 信封在特定的"域"上下文中签名，这是在创建和验证信封时指定的字符串
// 必须知道用于生成信封的域字符串才能验证签名并访问负载
type Envelope struct {
	// PublicKey 用于验证签名并派生签名者的节点ID的公钥
	PublicKey crypto.PubKey

	// PayloadType 指示负载中包含的数据类型的二进制标识符
	// TODO(yusef): 强制执行 multicodec 前缀
	PayloadType []byte

	// RawPayload 信封的负载数据
	RawPayload []byte

	// signature 域字符串 :: 类型提示 :: 负载的签名
	signature []byte

	// cached 作为 Record 缓存的未编组负载，首次通过 Record 访问器方法访问时缓存
	cached         Record
	unmarshalError error
	unmarshalOnce  sync.Once
}

// ErrEmptyDomain 表示信封域不能为空的错误
var ErrEmptyDomain = errors.New("信封域不能为空")

// ErrEmptyPayloadType 表示负载类型不能为空的错误
var ErrEmptyPayloadType = errors.New("负载类型不能为空")

// ErrInvalidSignature 表示签名无效或域不正确的错误
var ErrInvalidSignature = errors.New("无效的签名或不正确的域")

// Seal 将给定的 Record 编组，将编组的字节放入信封中，并使用给定的私钥签名
// 参数:
//   - rec: Record 要封装的记录
//   - privateKey: crypto.PrivKey 用于签名的私钥
//
// 返回值:
//   - *Envelope: 封装并签名后的信封
//   - error: 如果发生错误，返回错误信息
func Seal(rec Record, privateKey crypto.PrivKey) (*Envelope, error) {
	// 将记录编组为字节
	payload, err := rec.MarshalRecord()
	if err != nil {
		log.Debugf("序列化失败: %v", err)
		return nil, err
	}

	// 获取记录的域和负载类型
	domain := rec.Domain()
	payloadType := rec.Codec()
	if domain == "" {
		log.Debugf("域不能为空")
		return nil, ErrEmptyDomain
	}

	if len(payloadType) == 0 {
		log.Debugf("负载类型不能为空")
		return nil, ErrEmptyPayloadType
	}

	// 创建未签名的数据
	unsigned, err := makeUnsigned(domain, payloadType, payload)
	if err != nil {
		log.Debugf("创建未签名数据失败: %v", err)
		return nil, err
	}
	defer pool.Put(unsigned)

	// 使用私钥签名
	sig, err := privateKey.Sign(unsigned)
	if err != nil {
		log.Debugf("签名失败: %v", err)
		return nil, err
	}

	// 创建并返回新的信封
	return &Envelope{
		PublicKey:   privateKey.GetPublic(),
		PayloadType: payloadType,
		RawPayload:  payload,
		signature:   sig,
	}, nil
}

// ConsumeEnvelope 反序列化序列化的信封并使用提供的"域"字符串验证其签名
// 如果验证失败，将返回错误以及反序列化的信封，以便可以检查它
// 参数:
//   - data: []byte 序列化的信封数据
//   - domain: string 用于验证签名的域字符串
//
// 返回值:
//   - *Envelope: 反序列化的信封
//   - Record: 解析为具体记录类型的内部负载
//   - error: 如果发生错误，返回错误信息
func ConsumeEnvelope(data []byte, domain string) (envelope *Envelope, rec Record, err error) {
	// 反序列化信封
	e, err := UnmarshalEnvelope(data)
	if err != nil {
		log.Debugf("反序列化失败: %v", err)
		return nil, nil, err
	}

	// 验证信封
	err = e.validate(domain)
	if err != nil {
		log.Debugf("验证失败: %v", err)
		return nil, nil, err
	}

	// 获取记录
	rec, err = e.Record()
	if err != nil {
		log.Debugf("反序列化失败: %v", err)
		return nil, nil, err
	}
	return e, rec, nil
}

// ConsumeTypedEnvelope 反序列化序列化的信封并验证其签名
// 参数:
//   - data: []byte 序列化的信封数据
//   - destRecord: Record 目标记录实例，用于反序列化信封负载
//
// 返回值:
//   - *Envelope: 反序列化的信封
//   - error: 如果发生错误，返回错误信息
func ConsumeTypedEnvelope(data []byte, destRecord Record) (envelope *Envelope, err error) {
	// 反序列化信封
	e, err := UnmarshalEnvelope(data)
	if err != nil {
		log.Debugf("反序列化失败: %v", err)
		return nil, err
	}

	// 验证信封
	err = e.validate(destRecord.Domain())
	if err != nil {
		log.Debugf("验证失败: %v", err)
		return e, err
	}

	// 反序列化负载到目标记录
	err = destRecord.UnmarshalRecord(e.RawPayload)
	if err != nil {
		log.Debugf("反序列化失败: %v", err)
		return e, err
	}
	e.cached = destRecord
	return e, nil
}

// UnmarshalEnvelope 反序列化序列化的信封 protobuf 消息，而不验证其内容
// 参数:
//   - data: []byte 序列化的信封数据
//
// 返回值:
//   - *Envelope: 反序列化的信封
//   - error: 如果发生错误，返回错误信息
func UnmarshalEnvelope(data []byte) (*Envelope, error) {
	var e pb.Envelope
	if err := proto.Unmarshal(data, &e); err != nil {
		log.Debugf("反序列化失败: %v", err)
		return nil, err
	}

	key, err := crypto.PublicKeyFromProto(e.PublicKey)
	if err != nil {
		log.Debugf("反序列化失败: %v", err)
		return nil, err
	}

	return &Envelope{
		PublicKey:   key,
		PayloadType: e.PayloadType,
		RawPayload:  e.Payload,
		signature:   e.Signature,
	}, nil
}

// Marshal 返回包含信封序列化 protobuf 表示的字节切片
// 返回值:
//   - []byte: 序列化的信封数据
//   - error: 如果发生错误，返回错误信息
func (e *Envelope) Marshal() (res []byte, err error) {
	defer func() { catch.HandlePanic(recover(), &err, "dep2p envelope marshal") }()
	key, err := crypto.PublicKeyToProto(e.PublicKey)
	if err != nil {
		log.Debugf("序列化失败: %v", err)
		return nil, err
	}

	msg := pb.Envelope{
		PublicKey:   key,
		PayloadType: e.PayloadType,
		Payload:     e.RawPayload,
		Signature:   e.signature,
	}
	return proto.Marshal(&msg)
}

// Equal 如果另一个信封具有相同的公钥、负载、负载类型和签名，则返回 true
// 这意味着它们也是使用相同的域字符串创建的
// 参数:
//   - other: *Envelope 要比较的另一个信封
//
// 返回值:
//   - bool: 如果信封相等返回 true，否则返回 false
func (e *Envelope) Equal(other *Envelope) bool {
	if other == nil {
		return e == nil
	}
	return e.PublicKey.Equals(other.PublicKey) &&
		bytes.Equal(e.PayloadType, other.PayloadType) &&
		bytes.Equal(e.signature, other.signature) &&
		bytes.Equal(e.RawPayload, other.RawPayload)
}

// Record 返回作为 Record 反序列化的信封负载
// 返回的 Record 的具体类型取决于为信封的 PayloadType 注册的 Record 类型
// 一旦反序列化，Record 将被缓存以供将来访问
// 返回值:
//   - Record: 反序列化的记录
//   - error: 如果发生错误，返回错误信息
func (e *Envelope) Record() (Record, error) {
	e.unmarshalOnce.Do(func() {
		if e.cached != nil {
			return
		}
		e.cached, e.unmarshalError = unmarshalRecordPayload(e.PayloadType, e.RawPayload)
	})
	return e.cached, e.unmarshalError
}

// TypedRecord 将信封的负载反序列化到给定的 Record 实例中
// 参数:
//   - dest: Record 目标记录实例
//
// 返回值:
//   - error: 如果发生错误，返回错误信息
func (e *Envelope) TypedRecord(dest Record) error {
	return dest.UnmarshalRecord(e.RawPayload)
}

// validate 如果信封签名对给定的"域"有效，则返回 nil，如果签名验证失败，则返回错误
// 参数:
//   - domain: string 用于验证签名的域字符串
//
// 返回值:
//   - error: 如果验证失败，返回错误信息
func (e *Envelope) validate(domain string) error {
	unsigned, err := makeUnsigned(domain, e.PayloadType, e.RawPayload)
	if err != nil {
		log.Debugf("创建未签名数据失败: %v", err)
		return err
	}
	defer pool.Put(unsigned)

	valid, err := e.PublicKey.Verify(unsigned, e.signature)
	if err != nil {
		log.Debugf("验证签名失败: %v", err)
		return err
	}
	if !valid {
		log.Debugf("签名无效")
		return ErrInvalidSignature
	}
	return nil
}

// makeUnsigned 是一个帮助函数，用于准备要签名或验证的缓冲区
// 它从池中返回一个字节切片。调用者必须将此切片返回到池中
// 参数:
//   - domain: string 域字符串
//   - payloadType: []byte 负载类型
//   - payload: []byte 负载数据
//
// 返回值:
//   - []byte: 未签名的数据
//   - error: 如果发生错误，返回错误信息
func makeUnsigned(domain string, payloadType []byte, payload []byte) ([]byte, error) {
	var (
		// 将域、负载类型和负载组合成字段数组
		fields = [][]byte{[]byte(domain), payloadType, payload}

		// 字段前缀为其长度作为无符号 varint
		// 在分配签名缓冲区之前计算长度，这样我们就知道为长度添加多少空间
		flen = make([][]byte, len(fields))
		size = 0
	)

	// 计算每个字段的长度并编码为 varint
	for i, f := range fields {
		l := len(f)
		flen[i] = varint.ToUvarint(uint64(l))
		size += l + len(flen[i])
	}

	// 从池中获取缓冲区
	b := pool.Get(size)

	// 将所有字段复制到缓冲区
	var s int
	for i, f := range fields {
		s += copy(b[s:], flen[i])
		s += copy(b[s:], f)
	}

	return b[:s], nil
}
