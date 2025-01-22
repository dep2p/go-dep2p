package record

import (
	"errors"
	"reflect"

	"github.com/dep2p/go-dep2p/core/internal/catch"
	logging "github.com/dep2p/log"
)

var (
	// ErrPayloadTypeNotRegistered 当信封的 PayloadType 与任何已注册的 Record 类型不匹配时,从 ConsumeEnvelope 返回此错误
	ErrPayloadTypeNotRegistered = errors.New("负载类型未注册")

	payloadTypeRegistry = make(map[string]reflect.Type)
)

var log = logging.Logger("core-record")

// Record 表示可以用作信封负载的数据类型。
// Record 接口定义了用于将 Record 类型编组和解组为字节切片的方法。
//
// Record 类型可以使用 RegisterType 函数"注册"为给定信封的默认类型。
// PayloadType。一旦 Record 类型被注册,当使用 ConsumeEnvelope 函数打开信封时,
// 将创建该类型的实例并用于解组具有已注册 PayloadType 的任何信封的负载。
//
// 要使用未注册的 Record 类型,请使用 ConsumeTypedEnvelope 并传入您希望将信封的负载解组到的 Record 类型实例。
type Record interface {

	// Domain 是签名和验证特定 Record 类型时使用的"签名域"。
	// Domain 字符串对于您的 Record 类型应该是唯一的,并且 Record 类型的所有实例必须具有相同的 Domain 字符串。
	Domain() string

	// Codec 是此记录类型的二进制标识符,最好是已注册的 multicodec (参见 https://github.com/multiformats/multicodec)。
	// 当 Record 放入信封时(参见 record.Seal),Codec 值将用作信封的 PayloadType。
	// 当信封稍后被解封时,PayloadType 将用于查找正确的 Record 类型以将信封负载解组到其中。
	Codec() []byte

	// MarshalRecord 将 Record 实例转换为 []byte,以便可以用作信封负载。
	MarshalRecord() ([]byte, error)

	// UnmarshalRecord 将 []byte 负载解组为特定 Record 类型的实例。
	UnmarshalRecord([]byte) error
}

// RegisterType 将二进制负载类型标识符与具体的 Record 类型关联。
// 这用于在使用 ConsumeEnvelope 时自动解组来自信封的 Record 负载,
// 并在调用 Seal 时自动编组 Records 并确定正确的 PayloadType。
//
// 调用者必须提供要注册的记录类型的实例,该实例必须是指针类型。
// 注册应在定义 Record 类型的包的 init 函数中完成:
//
//	package hello_record
//	import record "github.com/dep2p/go-dep2p/core/record"
//
//	func init() {
//	    record.RegisterType(&HelloRecord{})
//	}
//
//	type HelloRecord struct { } // 等等..
func RegisterType(prototype Record) {
	payloadTypeRegistry[string(prototype.Codec())] = getValueType(prototype)
}

// unmarshalRecordPayload 将负载字节解组为对应的 Record 类型实例
// 参数:
//   - payloadType: []byte 负载类型标识符
//   - payloadBytes: []byte 需要解组的负载数据
//
// 返回值:
//   - Record: 解组后的 Record 实例
//   - error: 如果发生错误，返回错误信息
func unmarshalRecordPayload(payloadType []byte, payloadBytes []byte) (_rec Record, err error) {
	// 使用 defer 处理 panic
	defer func() { catch.HandlePanic(recover(), &err, "dep2p envelope record unmarshal") }()

	// 根据负载类型创建空白 Record 实例
	rec, err := blankRecordForPayloadType(payloadType)
	if err != nil {
		return nil, err
	}
	// 将负载字节解组到 Record 实例中
	err = rec.UnmarshalRecord(payloadBytes)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// blankRecordForPayloadType 根据负载类型创建对应的空白 Record 实例
// 参数:
//   - payloadType: []byte 负载类型标识符
//
// 返回值:
//   - Record: 创建的空白 Record 实例
//   - error: 如果负载类型未注册，返回错误信息
func blankRecordForPayloadType(payloadType []byte) (Record, error) {
	// 从注册表中查找对应的类型
	valueType, ok := payloadTypeRegistry[string(payloadType)]
	if !ok {
		return nil, ErrPayloadTypeNotRegistered
	}

	// 创建该类型的新实例
	val := reflect.New(valueType)
	asRecord := val.Interface().(Record)
	return asRecord, nil
}

// getValueType 获取接口值的实际类型
// 参数:
//   - i: interface{} 需要获取类型的接口值
//
// 返回值:
//   - reflect.Type: 接口值的实际类型
func getValueType(i interface{}) reflect.Type {
	// 获取接口值的类型
	valueType := reflect.TypeOf(i)
	// 如果是指针类型，获取其指向的类型
	if valueType.Kind() == reflect.Ptr {
		valueType = valueType.Elem()
	}
	return valueType
}
