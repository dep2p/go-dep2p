package nat

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	logging "github.com/dep2p/log"

	"github.com/dep2p/go-dep2p/p2plib/nat"
)

// ErrNoMapping 表示地址没有映射
var ErrNoMapping = errors.New("未建立映射")

// 日志记录器
var log = logging.Logger("net-nat")

// MappingDuration 是默认的端口映射持续时间
// 端口映射每 (MappingDuration / 3) 更新一次
const MappingDuration = time.Minute

// CacheTime 是映射缓存外部地址的时间
const CacheTime = 15 * time.Second

// entry 表示映射条目
type entry struct {
	protocol string // 协议
	port     int    // 端口
}

// 用于测试中的模拟
var discoverGateway = nat.DiscoverGateway

// DiscoverNAT 在网络中查找 NAT 设备并返回可以管理端口映射的对象
// 参数:
//   - ctx: context.Context 上下文对象
//
// 返回值:
//   - *NAT: NAT 管理对象
//   - error: 错误信息
func DiscoverNAT(ctx context.Context) (*NAT, error) {
	// 发现 NAT 网关
	natInstance, err := discoverGateway(ctx)
	if err != nil {
		log.Debugf("发现 NAT 网关失败: %v", err)
		return nil, err
	}

	// 获取外部地址
	var extAddr netip.Addr
	extIP, err := natInstance.GetExternalAddress()
	if err == nil {
		extAddr, _ = netip.AddrFromSlice(extIP)
	}

	// 记录设备地址
	addr, err := natInstance.GetDeviceAddress()
	if err != nil {
		log.Debugf("发现网关地址错误: %v", err)
	} else {
		log.Debugf("发现网关地址: %s", addr)
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	nat := &NAT{
		nat:       natInstance,
		extAddr:   extAddr,
		mappings:  make(map[entry]int),
		ctx:       ctx,
		ctxCancel: cancel,
	}
	nat.refCount.Add(1)
	go func() {
		defer nat.refCount.Done()
		nat.background()
	}()
	return nat, nil
}

// NAT 是一个管理 NAT(网络地址转换器)中地址端口映射的对象
// 它是一个长期运行的服务,会定期更新端口映射,
// 并保持所有外部地址的最新列表
type NAT struct {
	natmu sync.Mutex
	nat   nat.NAT
	// NAT 的外部 IP,将定期更新(每个 CacheTime)
	extAddr netip.Addr

	refCount  sync.WaitGroup
	ctx       context.Context
	ctxCancel context.CancelFunc

	mappingmu sync.RWMutex // 保护 mappings
	closed    bool
	mappings  map[entry]int
}

// Close 关闭所有端口映射。NAT 不能再被使用
// 返回值:
//   - error: 错误信息
func (nat *NAT) Close() error {
	nat.mappingmu.Lock()
	nat.closed = true
	nat.mappingmu.Unlock()

	nat.ctxCancel()
	nat.refCount.Wait()
	return nil
}

// GetMapping 获取指定协议和端口的映射
// 参数:
//   - protocol: string 协议名称
//   - port: int 端口号
//
// 返回值:
//   - addr: netip.AddrPort 映射的外部地址和端口
//   - found: bool 是否找到映射
func (nat *NAT) GetMapping(protocol string, port int) (addr netip.AddrPort, found bool) {
	nat.mappingmu.Lock()
	defer nat.mappingmu.Unlock()

	if !nat.extAddr.IsValid() {
		return netip.AddrPort{}, false
	}
	extPort, found := nat.mappings[entry{protocol: protocol, port: port}]
	if !found {
		return netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(nat.extAddr, uint16(extPort)), true
}

// AddMapping 尝试在指定协议和内部端口上构建映射
// 它会阻塞直到映射建立。一旦添加,它会定期更新映射。
//
// 可能不会成功,映射可能随时间变化;
// NAT 设备可能不遵守我们的端口请求,甚至会欺骗。
// 参数:
//   - ctx: context.Context 上下文对象
//   - protocol: string 协议名称
//   - port: int 端口号
//
// 返回值:
//   - error: 错误信息
func (nat *NAT) AddMapping(ctx context.Context, protocol string, port int) error {
	switch protocol {
	case "tcp", "udp":
	default:
		log.Debugf("无效协议: %s", protocol)
		return fmt.Errorf("无效协议: %s", protocol)
	}

	nat.mappingmu.Lock()
	defer nat.mappingmu.Unlock()

	if nat.closed {
		log.Debugf("已关闭")
		return errors.New("已关闭")
	}

	// 同步执行一次,以便第一次映射立即完成,并在退出前完成
	// 让用户在理想情况下可以立即使用结果
	extPort := nat.establishMapping(ctx, protocol, port)
	nat.mappings[entry{protocol: protocol, port: port}] = extPort
	return nil
}

// RemoveMapping 移除端口映射
// 它会阻塞直到 NAT 移除映射
// 参数:
//   - ctx: context.Context 上下文对象
//   - protocol: string 协议名称
//   - port: int 端口号
//
// 返回值:
//   - error: 错误信息
func (nat *NAT) RemoveMapping(ctx context.Context, protocol string, port int) error {
	nat.mappingmu.Lock()
	defer nat.mappingmu.Unlock()

	switch protocol {
	case "tcp", "udp":
		e := entry{protocol: protocol, port: port}
		if _, ok := nat.mappings[e]; ok {
			delete(nat.mappings, e)
			return nat.nat.DeletePortMapping(ctx, protocol, port)
		}
		return errors.New("未知映射")
	default:
		return fmt.Errorf("无效协议: %s", protocol)
	}
}

// background 在后台运行,定期更新映射和地址
func (nat *NAT) background() {
	const mappingUpdate = MappingDuration / 3

	now := time.Now()
	nextMappingUpdate := now.Add(mappingUpdate)
	nextAddrUpdate := now.Add(CacheTime)

	// 不使用 ticker,因为我们不知道建立映射需要多长时间
	t := time.NewTimer(minTime(nextMappingUpdate, nextAddrUpdate).Sub(now))
	defer t.Stop()

	var in []entry
	var out []int // 端口号
	for {
		select {
		case now := <-t.C:
			if now.After(nextMappingUpdate) {
				in = in[:0]
				out = out[:0]
				nat.mappingmu.Lock()
				for e := range nat.mappings {
					in = append(in, e)
				}
				nat.mappingmu.Unlock()
				// 建立映射涉及网络请求
				// 不持有互斥锁,只保存端口
				for _, e := range in {
					out = append(out, nat.establishMapping(nat.ctx, e.protocol, e.port))
				}
				nat.mappingmu.Lock()
				for i, p := range in {
					if _, ok := nat.mappings[p]; !ok {
						continue // 条目可能已被删除
					}
					nat.mappings[p] = out[i]
				}
				nat.mappingmu.Unlock()
				nextMappingUpdate = time.Now().Add(mappingUpdate)
			}
			if now.After(nextAddrUpdate) {
				var extAddr netip.Addr
				extIP, err := nat.nat.GetExternalAddress()
				if err == nil {
					extAddr, _ = netip.AddrFromSlice(extIP)
				}
				nat.extAddr = extAddr
				nextAddrUpdate = time.Now().Add(CacheTime)
			}
			t.Reset(time.Until(minTime(nextAddrUpdate, nextMappingUpdate)))
		case <-nat.ctx.Done():
			nat.mappingmu.Lock()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			for e := range nat.mappings {
				delete(nat.mappings, e)
				nat.nat.DeletePortMapping(ctx, e.protocol, e.port)
			}
			nat.mappingmu.Unlock()
			return
		}
	}
}

// establishMapping 建立端口映射
// 参数:
//   - ctx: context.Context 上下文对象
//   - protocol: string 协议名称
//   - internalPort: int 内部端口号
//
// 返回值:
//   - externalPort: int 外部端口号
func (nat *NAT) establishMapping(ctx context.Context, protocol string, internalPort int) (externalPort int) {
	log.Debugf("尝试端口映射: %s/%d", protocol, internalPort)
	const comment = "dep2p"

	nat.natmu.Lock()
	var err error
	externalPort, err = nat.nat.AddPortMapping(ctx, protocol, internalPort, comment, MappingDuration)
	if err != nil {
		// 某些硬件不支持带超时的映射,所以尝试不带超时
		externalPort, err = nat.nat.AddPortMapping(ctx, protocol, internalPort, comment, 0)
	}
	nat.natmu.Unlock()

	if err != nil || externalPort == 0 {
		// TODO: log.Event
		if err != nil {
			log.Warnf("建立端口映射失败: %s", err)
		} else {
			log.Warnf("建立端口映射失败: 新端口 = 0")
		}
		// 如果映射失败我们不关闭,
		// 因为下次可能会成功
		return 0
	}

	log.Debugf("NAT 映射: %d --> %d (%s)", externalPort, internalPort, protocol)
	return externalPort
}

// minTime 返回两个时间中较早的一个
// 参数:
//   - a: time.Time 时间 a
//   - b: time.Time 时间 b
//
// 返回值:
//   - time.Time 较早的时间
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
