package tls

import (
	"context"
	"net"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// BenchmarkCertificateGeneration 基准测试证书生成
func BenchmarkCertificateGeneration(b *testing.B) {
	id, _ := identity.Generate()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GenerateCert(id)
	}
}

// BenchmarkPeerVerification 基准测试 PeerID 验证
func BenchmarkPeerVerification(b *testing.B) {
	id, _ := identity.Generate()
	cert, _ := GenerateCert(id)
	peerID := types.PeerID(id.PeerID())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VerifyPeerCertificate(cert.Certificate, peerID)
	}
}

// BenchmarkTLSHandshake 基准测试 TLS 握手
func BenchmarkTLSHandshake(b *testing.B) {
	server, _ := identity.Generate()
	client, _ := identity.Generate()

	serverTransport, _ := New(server)
	clientTransport, _ := New(client)

	serverPeer := types.PeerID(server.PeerID())
	clientPeer := types.PeerID(client.PeerID())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		serverConn, clientConn := net.Pipe()
		ctx := context.Background()

		go func() {
			_, _ = serverTransport.SecureInbound(ctx, serverConn, clientPeer)
		}()

		_, _ = clientTransport.SecureOutbound(ctx, clientConn, serverPeer)
		serverConn.Close()
		clientConn.Close()
	}
}
