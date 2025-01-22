// Package peer contains Protobuf and JSON serialization/deserialization methods for peer IDs.
package peer

import (
	"encoding"
	"encoding/json"
)

// Interface assertions commented out to avoid introducing hard dependencies to protobuf.
// var _ proto.Marshaler = (*ID)(nil)
// var _ proto.Unmarshaler = (*ID)(nil)
var _ json.Marshaler = (*ID)(nil)
var _ json.Unmarshaler = (*ID)(nil)

var _ encoding.BinaryMarshaler = (*ID)(nil)
var _ encoding.BinaryUnmarshaler = (*ID)(nil)
var _ encoding.TextMarshaler = (*ID)(nil)
var _ encoding.TextUnmarshaler = (*ID)(nil)

// Marshal 将 peer ID 序列化为字节切片
// 返回值:
//   - []byte: 序列化后的字节切片
//   - error: 如果发生错误，返回错误信息
func (id ID) Marshal() ([]byte, error) {
	return []byte(id), nil
}

// MarshalBinary 返回 peer ID 的二进制表示
// 返回值:
//   - []byte: peer ID 的二进制表示
//   - error: 如果发生错误，返回错误信息
func (id ID) MarshalBinary() ([]byte, error) {
	return id.Marshal()
}

// MarshalTo 将 peer ID 序列化并写入指定的字节切片
// 参数:
//   - data: []byte 目标字节切片
//
// 返回值:
//   - n: int 写入的字节数
//   - err: error 如果发生错误，返回错误信息
func (id ID) MarshalTo(data []byte) (n int, err error) {
	return copy(data, []byte(id)), nil
}

// Unmarshal 从字节切片反序列化并设置 peer ID
// 参数:
//   - data: []byte 包含 peer ID 数据的字节切片
//
// 返回值:
//   - error: 如果发生错误，返回错误信息
func (id *ID) Unmarshal(data []byte) (err error) {
	*id, err = IDFromBytes(data)
	return err
}

// UnmarshalBinary 从二进制表示设置 peer ID
// 参数:
//   - data: []byte peer ID 的二进制表示
//
// 返回值:
//   - error: 如果发生错误，返回错误信息
func (id *ID) UnmarshalBinary(data []byte) error {
	return id.Unmarshal(data)
}

// Size 返回 peer ID 的字节大小
// 返回值:
//   - int: peer ID 的字节长度
func (id ID) Size() int {
	return len([]byte(id))
}

// MarshalJSON 将 peer ID 序列化为 JSON 格式
// 返回值:
//   - []byte: JSON 格式的字节切片
//   - error: 如果发生错误，返回错误信息
func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON 从 JSON 格式反序列化并设置 peer ID
// 参数:
//   - data: []byte JSON 格式的字节切片
//
// 返回值:
//   - error: 如果发生错误，返回错误信息
func (id *ID) UnmarshalJSON(data []byte) (err error) {
	var v string
	if err = json.Unmarshal(data, &v); err != nil {
		return err
	}
	*id, err = Decode(v)
	return err
}

// MarshalText 返回 peer ID 的文本编码
// 返回值:
//   - []byte: 文本编码的字节切片
//   - error: 如果发生错误，返回错误信息
func (id ID) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

// UnmarshalText 从文本编码恢复 peer ID
// 参数:
//   - data: []byte peer ID 的文本编码
//
// 返回值:
//   - error: 如果发生错误，返回错误信息
func (id *ID) UnmarshalText(data []byte) error {
	pid, err := Decode(string(data))
	if err != nil {
		log.Debugf("反序列化失败: %v", err)
		return err
	}
	*id = pid
	return nil
}
