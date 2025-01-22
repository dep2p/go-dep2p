package peer

import (
	"encoding/json"

	"github.com/dep2p/go-dep2p/core/internal/catch"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// addrInfoJson 用于JSON序列化和反序列化的辅助结构体
// 由于无法直接将JSON反序列化到接口类型(Multiaddr)，需要使用该结构体作为中间层
type addrInfoJson struct {
	ID    ID       // 节点标识符
	Addrs []string // 地址列表，使用字符串表示
}

// MarshalJSON 将AddrInfo实例序列化为JSON格式
// 返回值:
//   - []byte: 序列化后的JSON字节数组
//   - error: 如果发生错误，返回错误信息
func (pi AddrInfo) MarshalJSON() (res []byte, err error) {
	// 使用defer处理可能发生的panic
	defer func() { catch.HandlePanic(recover(), &err, "dep2p addr info marshal") }()

	// 将Multiaddr地址列表转换为字符串数组
	addrs := make([]string, len(pi.Addrs))
	for i, addr := range pi.Addrs {
		addrs[i] = addr.String()
	}

	// 将数据序列化为JSON格式
	return json.Marshal(&addrInfoJson{
		ID:    pi.ID,
		Addrs: addrs,
	})
}

// UnmarshalJSON 从JSON数据反序列化创建AddrInfo实例
// 参数:
//   - b: []byte JSON格式的字节数组
//
// 返回值:
//   - error: 如果发生错误，返回错误信息
func (pi *AddrInfo) UnmarshalJSON(b []byte) (err error) {
	// 使用defer处理可能发生的panic
	defer func() { catch.HandlePanic(recover(), &err, "dep2p addr info unmarshal") }()

	// 解析JSON数据到临时结构体
	var data addrInfoJson
	if err := json.Unmarshal(b, &data); err != nil {
		log.Debugf("反序列化失败: %v", err)
		return err
	}

	// 将字符串地址转换为Multiaddr类型
	addrs := make([]ma.Multiaddr, len(data.Addrs))
	for i, addr := range data.Addrs {
		maddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			log.Debugf("创建多地址失败: %v", err)
			return err
		}
		addrs[i] = maddr
	}

	// 更新AddrInfo实例的字段
	pi.ID = data.ID
	pi.Addrs = addrs
	return nil
}
