package peerstore

import (
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestPeerstore_ConcurrentAddAddrs(t *testing.T) {
	ps := NewPeerstore()
	peerID := testPeerID("peer1")

	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				addr := testMultiaddr("/ip4/127.0.0.1/tcp/4001")
				ps.AddAddrs(peerID, []types.Multiaddr{addr}, ConnectedAddrTTL)
			}
		}(i)
	}

	wg.Wait()

	// 验证没有崩溃即可
	addrs := ps.Addrs(peerID)
	assert.NotEmpty(t, addrs)
}

func TestPeerstore_ConcurrentAddPubKey(t *testing.T) {
	ps := NewPeerstore()

	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				peerID := testPeerID("peer" + string(rune(id)))
				pubKey := testPubKey("key" + string(rune(id)))
				ps.AddPubKey(peerID, pubKey)
			}
		}(i)
	}

	wg.Wait()

	// 验证没有崩溃即可
	peers := ps.Peers()
	assert.NotEmpty(t, peers)
}

func TestPeerstore_ConcurrentSetProtocols(t *testing.T) {
	ps := NewPeerstore()
	peerID := testPeerID("peer1")

	const goroutines = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			proto := testProtocolID("/dep2p/test/" + string(rune(id)))
			ps.SetProtocols(peerID, proto)
		}(i)
	}

	wg.Wait()

	// 验证没有崩溃即可
	protocols, _ := ps.GetProtocols(peerID)
	assert.NotEmpty(t, protocols)
}

func TestPeerstore_ConcurrentReadWrite(t *testing.T) {
	ps := NewPeerstore()
	peerID := testPeerID("peer1")

	// 先添加一些数据
	ps.AddAddrs(peerID, []types.Multiaddr{testMultiaddr("/ip4/127.0.0.1/tcp/4001")}, ConnectedAddrTTL)
	ps.AddPubKey(peerID, testPubKey("key1"))
	ps.SetProtocols(peerID, testProtocolID("/dep2p/sys/dht/1.0.0"))

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// 一半协程读
	for i := 0; i < goroutines/2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ps.Addrs(peerID)
				ps.PubKey(peerID)
				ps.GetProtocols(peerID)
				time.Sleep(time.Microsecond)
			}
		}()
	}

	// 一半协程写
	for i := 0; i < goroutines/2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ps.AddAddrs(peerID, []types.Multiaddr{testMultiaddr("/ip4/127.0.0.1/tcp/4002")}, ConnectedAddrTTL)
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()

	// 验证数据一致性
	addrs := ps.Addrs(peerID)
	assert.NotEmpty(t, addrs)
}
