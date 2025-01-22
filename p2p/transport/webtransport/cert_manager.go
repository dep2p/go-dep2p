package dep2pwebtransport

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	ic "github.com/dep2p/go-dep2p/core/crypto"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/dep2p/go-dep2p/multiformats/multihash"
)

// 允许一定的时钟偏差。
// 当我们生成证书时,NotBefore 时间设置为当前时间减去 clockSkewAllowance。
// 同样,我们在证书过期前的 clockSkewAllowance 时间就停止使用该证书。
const clockSkewAllowance = time.Hour
const validityMinusTwoSkew = certValidity - (2 * clockSkewAllowance)

// certConfig 证书配置结构体
type certConfig struct {
	tlsConf *tls.Config // TLS 配置
	sha256  [32]byte    // 缓存的 TLS 配置 SHA256 哈希值
}

// Start 获取证书的生效时间
// 返回值:
//   - time.Time 证书生效时间
func (c *certConfig) Start() time.Time { return c.tlsConf.Certificates[0].Leaf.NotBefore }

// End 获取证书的过期时间
// 返回值:
//   - time.Time 证书过期时间
func (c *certConfig) End() time.Time { return c.tlsConf.Certificates[0].Leaf.NotAfter }

// newCertConfig 创建新的证书配置
// 参数:
//   - key: ic.PrivKey 私钥
//   - start: time.Time 证书生效时间
//   - end: time.Time 证书过期时间
//
// 返回值:
//   - *certConfig 证书配置对象
//   - error 错误信息
func newCertConfig(key ic.PrivKey, start, end time.Time) (*certConfig, error) {
	conf, err := getTLSConf(key, start, end)
	if err != nil {
		log.Debugf("获取 TLS 配置失败: %s", err)
		return nil, err
	}
	return &certConfig{
		tlsConf: conf,
		sha256:  sha256.Sum256(conf.Certificates[0].Leaf.Raw),
	}, nil
}

// 证书更新逻辑:
//  1. 在启动时,我们生成一个从现在开始生效的证书(减去1小时以允许时钟偏差),以及另一个从第一个证书过期时开始生效的证书(同样考虑时钟偏差)。
//  2. 当第一个证书过期前1小时时,我们切换到第二个证书。
//     同时,我们停止广播第一个证书的哈希值并生成下一个证书。
type certManager struct {
	clock     clock.Clock        // 时钟对象
	ctx       context.Context    // 上下文
	ctxCancel context.CancelFunc // 取消函数
	refCount  sync.WaitGroup     // 引用计数

	mx            sync.RWMutex // 互斥锁
	lastConfig    *certConfig  // 上一个证书配置,初始为 nil
	currentConfig *certConfig  // 当前证书配置
	nextConfig    *certConfig  // 下一个证书配置,直到当前配置过半有效期才设置
	addrComp      ma.Multiaddr // 地址组件

	serializedCertHashes [][]byte // 序列化的证书哈希列表
}

// newCertManager 创建新的证书管理器
// 参数:
//   - hostKey: ic.PrivKey 主机私钥
//   - clock: clock.Clock 时钟对象
//
// 返回值:
//   - *certManager 证书管理器对象
//   - error 错误信息
func newCertManager(hostKey ic.PrivKey, clock clock.Clock) (*certManager, error) {
	m := &certManager{clock: clock}
	m.ctx, m.ctxCancel = context.WithCancel(context.Background())
	if err := m.init(hostKey); err != nil {
		log.Debugf("初始化证书管理器失败: %s", err)
		return nil, err
	}

	m.background(hostKey)
	return m, nil
}

// getCurrentBucketStartTime 获取给定时间在以 certValidity 为间隔的时间桶中的规范开始时间
// 这允许你在重启时获得相同的时间范围而无需持久化状态
// ```
// ... v--- epoch + offset
// ... |--------|    |--------|        ...
// ...        |--------|    |--------| ...
// ```
// 参数:
//   - now: time.Time 当前时间
//   - offset: time.Duration 时间偏移量
//
// 返回值:
//   - time.Time 时间桶的开始时间
func getCurrentBucketStartTime(now time.Time, offset time.Duration) time.Time {
	currentBucket := (now.UnixMilli() - offset.Milliseconds()) / validityMinusTwoSkew.Milliseconds()
	return time.UnixMilli(offset.Milliseconds() + currentBucket*validityMinusTwoSkew.Milliseconds())
}

// init 初始化证书管理器
// 参数:
//   - hostKey: ic.PrivKey 主机私钥
//
// 返回值:
//   - error 错误信息
func (m *certManager) init(hostKey ic.PrivKey) error {
	start := m.clock.Now()
	pubkeyBytes, err := hostKey.GetPublic().Raw()
	if err != nil {
		log.Debugf("获取公钥失败: %s", err)
		return err
	}

	// 我们想为每个开始时间添加一个随机偏移量,这样网络中的所有证书不会同时轮换
	// 偏移量表示将桶开始时间提前某个 offset
	offset := (time.Duration(binary.LittleEndian.Uint16(pubkeyBytes)) * time.Minute) % certValidity

	// 我们希望证书至少已经生效了一个 clockSkewAllowance 的时间
	start = start.Add(-clockSkewAllowance)
	startTime := getCurrentBucketStartTime(start, offset)
	m.nextConfig, err = newCertConfig(hostKey, startTime, startTime.Add(certValidity))
	if err != nil {
		log.Debugf("创建下一个证书配置失败: %s", err)
		return err
	}
	return m.rollConfig(hostKey)
}

// rollConfig 轮换证书配置
// 参数:
//   - hostKey: ic.PrivKey 主机私钥
//
// 返回值:
//   - error 错误信息
func (m *certManager) rollConfig(hostKey ic.PrivKey) error {
	// 我们在证书过期前的 clockSkewAllowance 时间停止使用当前证书
	// 此时,下一个证书需要已经生效一个 clockSkewAllowance 的时间
	nextStart := m.nextConfig.End().Add(-2 * clockSkewAllowance)
	c, err := newCertConfig(hostKey, nextStart, nextStart.Add(certValidity))
	if err != nil {
		log.Debugf("创建下一个证书配置失败: %s", err)
		return err
	}
	m.lastConfig = m.currentConfig
	m.currentConfig = m.nextConfig
	m.nextConfig = c
	if err := m.cacheSerializedCertHashes(); err != nil {
		log.Debugf("缓存序列化证书哈希失败: %s", err)
		return err
	}
	return m.cacheAddrComponent()
}

// background 在后台运行证书轮换
// 参数:
//   - hostKey: ic.PrivKey 主机私钥
func (m *certManager) background(hostKey ic.PrivKey) {
	d := m.currentConfig.End().Add(-clockSkewAllowance).Sub(m.clock.Now())
	log.Debugw("设置定时器", "duration", d.String())
	t := m.clock.Timer(d)
	m.refCount.Add(1)

	go func() {
		defer m.refCount.Done()
		defer t.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-t.C:
				now := m.clock.Now()
				m.mx.Lock()
				if err := m.rollConfig(hostKey); err != nil {
					log.Debugf("轮换配置失败: %s", err)
				}
				d := m.currentConfig.End().Add(-clockSkewAllowance).Sub(now)
				log.Debugw("轮换证书", "next", d.String())
				t.Reset(d)
				m.mx.Unlock()
			}
		}
	}()
}

// GetConfig 获取当前的 TLS 配置
// 返回值:
//   - *tls.Config TLS 配置对象
func (m *certManager) GetConfig() *tls.Config {
	m.mx.RLock()
	defer m.mx.RUnlock()
	return m.currentConfig.tlsConf
}

// AddrComponent 获取地址组件
// 返回值:
//   - ma.Multiaddr 地址组件
func (m *certManager) AddrComponent() ma.Multiaddr {
	m.mx.RLock()
	defer m.mx.RUnlock()
	return m.addrComp
}

// SerializedCertHashes 获取序列化的证书哈希列表
// 返回值:
//   - [][]byte 证书哈希列表
func (m *certManager) SerializedCertHashes() [][]byte {
	return m.serializedCertHashes
}

// cacheSerializedCertHashes 缓存序列化的证书哈希
// 返回值:
//   - error 错误信息
func (m *certManager) cacheSerializedCertHashes() error {
	hashes := make([][32]byte, 0, 3)
	if m.lastConfig != nil {
		hashes = append(hashes, m.lastConfig.sha256)
	}
	hashes = append(hashes, m.currentConfig.sha256)
	if m.nextConfig != nil {
		hashes = append(hashes, m.nextConfig.sha256)
	}

	m.serializedCertHashes = m.serializedCertHashes[:0]
	for _, certHash := range hashes {
		h, err := multihash.Encode(certHash[:], multihash.SHA2_256)
		if err != nil {
			log.Debugf("编码证书哈希失败: %s", err)
			return err
		}
		m.serializedCertHashes = append(m.serializedCertHashes, h)
	}
	return nil
}

// cacheAddrComponent 缓存地址组件
// 返回值:
//   - error 错误信息
func (m *certManager) cacheAddrComponent() error {
	addr, err := addrComponentForCert(m.currentConfig.sha256[:])
	if err != nil {
		log.Debugf("获取地址组件失败: %s", err)
		return err
	}
	if m.nextConfig != nil {
		comp, err := addrComponentForCert(m.nextConfig.sha256[:])
		if err != nil {
			log.Debugf("获取下一个地址组件失败: %s", err)
			return err
		}
		addr = addr.Encapsulate(comp)
	}
	m.addrComp = addr
	return nil
}

// Close 关闭证书管理器
// 返回值:
//   - error 错误信息
func (m *certManager) Close() error {
	m.ctxCancel()
	m.refCount.Wait()
	return nil
}
