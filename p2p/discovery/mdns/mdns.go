package mdns

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"strings"
	"sync"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"

	"github.com/dep2p/go-dep2p/p2plib/zeroconf"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	logging "github.com/dep2p/log"
)

// 常量定义
const (
	ServiceName   = "_p2p._udp" // mDNS服务名称
	mdnsDomain    = "local"     // mDNS域名
	dnsaddrPrefix = "dnsaddr="  // DNS地址前缀
)

// log 用于记录mDNS相关的日志
var log = logging.Logger("discovery-mdns")

// Service 定义了mDNS服务的接口
type Service interface {
	// Start 启动mDNS服务
	// 返回值:
	//   - error: 如果启动失败返回错误信息
	Start() error
	io.Closer
}

// Notifee 定义了处理发现节点的接口
type Notifee interface {
	// HandlePeerFound 处理发现的新节点
	// 参数:
	//   - peer.AddrInfo: 发现的节点信息
	HandlePeerFound(peer.AddrInfo)
}

// mdnsService 实现了mDNS服务
type mdnsService struct {
	host        host.Host // dep2p主机实例
	serviceName string    // 服务名称
	peerName    string    // 节点名称

	// ctx在调用Close()时被取消
	ctx       context.Context    // 上下文
	ctxCancel context.CancelFunc // 取消函数

	resolverWG sync.WaitGroup   // 用于等待解析器goroutine完成
	server     *zeroconf.Server // zeroconf服务器实例

	notifee Notifee // 节点发现通知接口
}

// NewMdnsService 创建一个新的mDNS服务实例
// 参数:
//   - host: dep2p主机实例
//   - serviceName: 服务名称,如果为空则使用默认值
//   - notifee: 节点发现通知接口
//
// 返回值:
//   - *mdnsService: 返回创建的mDNS服务实例
func NewMdnsService(host host.Host, serviceName string, notifee Notifee) *mdnsService {
	// 如果服务名为空,使用默认值
	if serviceName == "" {
		serviceName = ServiceName
	}
	// 创建服务实例
	s := &mdnsService{
		host:        host,
		serviceName: serviceName,
		peerName:    randomString(32 + rand.Intn(32)), // 生成32-63字符长度的随机字符串
		notifee:     notifee,
	}
	// 创建上下文和取消函数
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	return s
}

// Start 启动mDNS服务
// 返回值:
//   - error: 如果启动失败返回错误信息
func (s *mdnsService) Start() error {
	// 启动服务器
	if err := s.startServer(); err != nil {
		log.Debugf("启动mDNS服务器失败: %v", err)
		return err
	}
	// 启动解析器
	s.startResolver(s.ctx)
	return nil
}

// Close 关闭mDNS服务
// 返回值:
//   - error: 如果关闭失败返回错误信息
func (s *mdnsService) Close() error {
	// 取消上下文
	s.ctxCancel()
	// 关闭服务器
	if s.server != nil {
		s.server.Shutdown()
	}
	// 等待解析器goroutine完成
	s.resolverWG.Wait()
	return nil
}

// getIPs 获取IP地址列表
// 虽然我们不太关心IP地址,但规范(以及各种路由器/防火墙)要求我们发送A和AAAA记录
// 参数:
//   - addrs: 多地址列表
//
// 返回值:
//   - []string: 返回IP地址列表
//   - error: 如果获取失败返回错误信息
func (s *mdnsService) getIPs(addrs []ma.Multiaddr) ([]string, error) {
	var ip4, ip6 string
	// 遍历多地址列表
	for _, addr := range addrs {
		first, _ := ma.SplitFirst(addr)
		if first == nil {
			continue
		}
		// 获取IPv4和IPv6地址
		if ip4 == "" && first.Protocol().Code == ma.P_IP4 {
			ip4 = first.Value()
		} else if ip6 == "" && first.Protocol().Code == ma.P_IP6 {
			ip6 = first.Value()
		}
	}
	// 构建IP地址列表
	ips := make([]string, 0, 2)
	if ip4 != "" {
		ips = append(ips, ip4)
	}
	if ip6 != "" {
		ips = append(ips, ip6)
	}
	if len(ips) == 0 {
		log.Debugf("没有找到任何IP地址")
		return nil, errors.New("没有找到任何IP地址")
	}
	return ips, nil
}

// startServer 启动mDNS服务器
// 返回值:
//   - error: 如果启动失败返回错误信息
func (s *mdnsService) startServer() error {
	// 获取接口监听地址
	interfaceAddrs, err := s.host.Network().InterfaceListenAddresses()
	if err != nil {
		log.Debugf("获取接口监听地址失败: %v", err)
		return err
	}
	// 转换为p2p地址
	addrs, err := peer.AddrInfoToP2pAddrs(&peer.AddrInfo{
		ID:    s.host.ID(),
		Addrs: interfaceAddrs,
	})
	if err != nil {
		log.Debugf("转换为p2p地址失败: %v", err)
		return err
	}
	// 构建TXT记录
	var txts []string
	for _, addr := range addrs {
		if manet.IsThinWaist(addr) { // 不广播中继地址
			txts = append(txts, dnsaddrPrefix+addr.String())
		}
	}

	// 获取IP地址
	ips, err := s.getIPs(addrs)
	if err != nil {
		log.Debugf("获取IP地址失败: %v", err)
		return err
	}

	// 注册zeroconf服务
	server, err := zeroconf.RegisterProxy(
		s.peerName,
		s.serviceName,
		mdnsDomain,
		4001, // 必须传入端口号,但dep2p只使用TXT记录
		s.peerName,
		ips,
		txts,
		nil,
	)
	if err != nil {
		log.Debugf("注册zeroconf服务失败: %v", err)
		return err
	}
	s.server = server
	return nil
}

// startResolver 启动mDNS解析器
// 参数:
//   - ctx: 上下文
func (s *mdnsService) startResolver(ctx context.Context) {
	s.resolverWG.Add(2)
	entryChan := make(chan *zeroconf.ServiceEntry, 1000)
	// 启动处理服务条目的goroutine
	go func() {
		defer s.resolverWG.Done()
		for entry := range entryChan {
			// 我们只关心TXT记录
			// 忽略A、AAAA和PTR记录
			addrs := make([]ma.Multiaddr, 0, len(entry.Text)) // 假设所有TXT记录都是dnsaddr
			for _, s := range entry.Text {
				if !strings.HasPrefix(s, dnsaddrPrefix) {
					log.Debugf("缺少dnsaddr前缀: %s", s)
					continue
				}
				addr, err := ma.NewMultiaddr(s[len(dnsaddrPrefix):])
				if err != nil {
					log.Debugf("解析多地址失败: %s", err)
					continue
				}
				addrs = append(addrs, addr)
			}
			infos, err := peer.AddrInfosFromP2pAddrs(addrs...)
			if err != nil {
				log.Debugf("获取节点信息失败: %s", err)
				continue
			}
			for _, info := range infos {
				if info.ID == s.host.ID() {
					continue
				}
				go s.notifee.HandlePeerFound(info)
			}
		}
	}()
	// 启动浏览服务的goroutine
	go func() {
		defer s.resolverWG.Done()
		if err := zeroconf.Browse(ctx, s.serviceName, mdnsDomain, entryChan); err != nil {
			log.Debugf("zeroconf浏览失败: %s", err)
		}
	}()
}

// randomString 生成指定长度的随机字符串
// 参数:
//   - l: 字符串长度
//
// 返回值:
//   - string: 返回生成的随机字符串
func randomString(l int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	s := make([]byte, 0, l)
	for i := 0; i < l; i++ {
		s = append(s, alphabet[rand.Intn(len(alphabet))])
	}
	return string(s)
}
