// Package upgrader 实现连接升级器
package upgrader

import (
	"context"
	"fmt"
	"net"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	mss "github.com/multiformats/go-multistream"
)

const (
	// defaultNegotiateTimeout 默认协商超时
	defaultNegotiateTimeout = 60 * time.Second
)

// negotiateSecurity 协商安全协议
//
// 使用 multistream-select 协议在客户端和服务器之间协商安全协议。
// 服务器端使用 MultistreamMuxer.Negotiate()
// 客户端使用 SelectOneOf()
func (u *Upgrader) negotiateSecurity(ctx context.Context, conn net.Conn, isServer bool) (pkgif.SecureTransport, error) {
	// 设置协商超时
	deadline := time.Now().Add(defaultNegotiateTimeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}
	defer conn.SetDeadline(time.Time{}) // 清除超时

	var selectedProto string
	var err error

	if isServer {
		// 服务器端：从客户端提议中选择
		muxer := mss.NewMultistreamMuxer[string]()
		for _, st := range u.securityTransports {
			protoID := string(st.ID())
			muxer.AddHandler(protoID, nil)
		}

		selectedProto, _, err = muxer.Negotiate(conn)
		if err != nil {
			return nil, fmt.Errorf("server security negotiation: %w", err)
		}
	} else {
		// 客户端：提议协议列表
		protocols := make([]string, len(u.securityTransports))
		for i, st := range u.securityTransports {
			protocols[i] = string(st.ID())
		}

		selectedProto, err = mss.SelectOneOf(protocols, conn)
		if err != nil {
			return nil, fmt.Errorf("client security negotiation: %w", err)
		}
	}

	// 找到对应的 SecureTransport
	for _, st := range u.securityTransports {
		if string(st.ID()) == selectedProto {
			return st, nil
		}
	}

	return nil, fmt.Errorf("negotiated protocol %s not found", selectedProto)
}

// negotiateMuxer 协商多路复用器
//
// 使用 multistream-select 协议在客户端和服务器之间协商多路复用器。
func (u *Upgrader) negotiateMuxer(ctx context.Context, conn net.Conn, isServer bool) (pkgif.StreamMuxer, error) {
	// 设置协商超时
	deadline := time.Now().Add(defaultNegotiateTimeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}
	defer conn.SetDeadline(time.Time{}) // 清除超时

	var selectedProto string
	var err error

	if isServer {
		// 服务器端：从客户端提议中选择
		muxer := mss.NewMultistreamMuxer[string]()
		for _, sm := range u.streamMuxers {
			protoID := sm.ID()
			muxer.AddHandler(protoID, nil)
		}

		selectedProto, _, err = muxer.Negotiate(conn)
		if err != nil {
			return nil, fmt.Errorf("server muxer negotiation: %w", err)
		}
	} else {
		// 客户端：提议协议列表
		protocols := make([]string, len(u.streamMuxers))
		for i, sm := range u.streamMuxers {
			protocols[i] = sm.ID()
		}

		selectedProto, err = mss.SelectOneOf(protocols, conn)
		if err != nil {
			return nil, fmt.Errorf("client muxer negotiation: %w", err)
		}
	}

	// 找到对应的 StreamMuxer
	for _, sm := range u.streamMuxers {
		if sm.ID() == selectedProto {
			return sm, nil
		}
	}

	return nil, fmt.Errorf("negotiated muxer %s not found", selectedProto)
}
