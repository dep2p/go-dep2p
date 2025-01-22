package connmgr

import (
	"context"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
)

// NullConnMgr 是一个不提供任何功能的 ConnMgr。
type NullConnMgr struct{}

var _ ConnManager = (*NullConnMgr)(nil)

func (NullConnMgr) TagPeer(peer.ID, string, int)             {}
func (NullConnMgr) UntagPeer(peer.ID, string)                {}
func (NullConnMgr) UpsertTag(peer.ID, string, func(int) int) {}
func (NullConnMgr) GetTagInfo(peer.ID) *TagInfo              { return &TagInfo{} }
func (NullConnMgr) TrimOpenConns(ctx context.Context)        {}
func (NullConnMgr) Notifee() network.Notifiee                { return network.GlobalNoopNotifiee }
func (NullConnMgr) Protect(peer.ID, string)                  {}
func (NullConnMgr) Unprotect(peer.ID, string) bool           { return false }
func (NullConnMgr) IsProtected(peer.ID, string) bool         { return false }
func (NullConnMgr) CheckLimit(l GetConnLimiter) error        { return nil }
func (NullConnMgr) Close() error                             { return nil }
