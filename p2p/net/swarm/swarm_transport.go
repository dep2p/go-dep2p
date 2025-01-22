package swarm

import (
	"fmt"
	"strings"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// TransportForDialing 获取用于拨号给定多地址的适当传输层
// 参数:
//   - a: ma.Multiaddr 多地址对象
//
// 返回值:
//   - transport.Transport 传输层对象
//
// 注意:
//   - 如果没有找到合适的传输层,返回 nil
func (s *Swarm) TransportForDialing(a ma.Multiaddr) transport.Transport {
	// 检查多地址是否为空
	if a == nil {
		return nil
	}
	// 获取多地址的协议列表
	protocols := a.Protocols()
	// 如果协议列表为空,返回 nil
	if len(protocols) == 0 {
		return nil
	}

	// 加读锁
	s.transports.RLock()
	defer s.transports.RUnlock()

	// 检查是否有可用的传输层
	if len(s.transports.m) == 0 {
		// 确认不是正在关闭
		if s.transports.m != nil {
			log.Debugf("未配置任何传输层")
		}
		return nil
	}
	// 如果是中继地址,返回中继传输层
	if isRelayAddr(a) {
		return s.transports.m[ma.P_CIRCUIT]
	}
	// 尝试从地址中提取对等点 ID
	if id, _ := peer.IDFromP2PAddr(a); id != "" {
		// 该地址包含 p2p 组件,移除它以检查传输层
		a, _ = ma.SplitLast(a)
		if a == nil {
			return nil
		}
	}
	// 遍历所有传输层,返回第一个可以拨号的传输层
	for _, t := range s.transports.m {
		if t.CanDial(a) {
			return t
		}
	}
	return nil
}

// TransportForListening 获取用于监听给定多地址的适当传输层
// 参数:
//   - a: ma.Multiaddr 多地址对象
//
// 返回值:
//   - transport.Transport 传输层对象
//
// 注意:
//   - 如果没有找到合适的传输层,返回 nil
func (s *Swarm) TransportForListening(a ma.Multiaddr) transport.Transport {
	// 获取多地址的协议列表
	protocols := a.Protocols()
	// 如果协议列表为空,返回 nil
	if len(protocols) == 0 {
		return nil
	}

	// 加读锁
	s.transports.RLock()
	defer s.transports.RUnlock()
	// 检查是否有可用的传输层
	if len(s.transports.m) == 0 {
		// 确认不是正在关闭
		if s.transports.m != nil {
			log.Debugf("未配置任何传输层")
		}
		return nil
	}

	// 默认选择最后一个协议对应的传输层
	selected := s.transports.m[protocols[len(protocols)-1].Code]
	// 遍历所有协议
	for _, p := range protocols {
		transport, ok := s.transports.m[p.Code]
		if !ok {
			continue
		}
		// 如果是代理传输层,则选择它
		if transport.Proxy() {
			selected = transport
		}
	}
	return selected
}

// AddTransport 向 swarm 添加一个传输层
// 参数:
//   - t: transport.Transport 传输层对象
//
// 返回值:
//   - error 错误信息
//
// 注意:
//   - 满足 go-dep2p-transport 的 Network 接口
func (s *Swarm) AddTransport(t transport.Transport) error {
	// 获取传输层支持的协议列表
	protocols := t.Protocols()

	// 如果协议列表为空,返回错误
	if len(protocols) == 0 {
		log.Debugf("无用的传输层不处理任何协议: %T", t)
		return fmt.Errorf("无用的传输层不处理任何协议: %T", t)
	}

	// 加写锁
	s.transports.Lock()
	defer s.transports.Unlock()
	// 如果传输层映射为空,说明已关闭
	if s.transports.m == nil {
		log.Debugf("swarm已关闭")
		return ErrSwarmClosed
	}
	// 检查是否有协议已被注册
	var registered []string
	for _, p := range protocols {
		if _, ok := s.transports.m[p]; ok {
			proto := ma.ProtocolWithCode(p)
			name := proto.Name
			if name == "" {
				name = fmt.Sprintf("未知 (%d)", p)
			}
			registered = append(registered, name)
		}
	}
	// 如果有协议已被注册,返回错误
	if len(registered) > 0 {
		log.Debugf("以下协议已注册传输层: %s", strings.Join(registered, ", "))
		return fmt.Errorf(
			"以下协议已注册传输层: %s",
			strings.Join(registered, ", "),
		)
	}

	// 注册所有协议的传输层
	for _, p := range protocols {
		s.transports.m[p] = t
	}
	return nil
}
