package routing

import (
	"encoding/json"

	"github.com/dep2p/go-dep2p/core/peer"
)

// MarshalJSON 将 QueryEvent 对象序列化为 JSON 字节数组
// 返回值：
//   - []byte: 序列化后的 JSON 字节数组
//   - error: 如果序列化过程中发生错误，返回错误信息
func (qe *QueryEvent) MarshalJSON() ([]byte, error) {
	// 将 QueryEvent 对象转换为 map 并序列化为 JSON
	return json.Marshal(map[string]interface{}{
		"ID":        qe.ID.String(), // 将 PeerID 转换为字符串
		"Type":      int(qe.Type),   // 将查询事件类型转换为整数
		"Responses": qe.Responses,   // 包含响应的节点地址信息
		"Extra":     qe.Extra,       // 额外的自定义数据
	})
}

// UnmarshalJSON 从 JSON 字节数组反序列化得到 QueryEvent 对象
// 参数：
//   - b: []byte JSON 格式的字节数组
//
// 返回值：
//   - error: 如果反序列化过程中发生错误，返回错误信息
func (qe *QueryEvent) UnmarshalJSON(b []byte) error {
	// 定义临时结构体用于解析 JSON 数据
	temp := struct {
		ID        string
		Type      int
		Responses []*peer.AddrInfo
		Extra     string
	}{}

	// 将 JSON 数据解析到临时结构体
	err := json.Unmarshal(b, &temp)
	if err != nil {
		log.Debugf("反序列化失败: %v", err)
		return err
	}

	// 如果 ID 字段不为空，则解码为 PeerID
	if len(temp.ID) > 0 {
		pid, err := peer.Decode(temp.ID)
		if err != nil {
			log.Debugf("解码失败: %v", err)
			return err
		}
		qe.ID = pid
	}

	// 将解析后的数据赋值给 QueryEvent 对象
	qe.Type = QueryEventType(temp.Type)
	qe.Responses = temp.Responses
	qe.Extra = temp.Extra
	return nil
}
