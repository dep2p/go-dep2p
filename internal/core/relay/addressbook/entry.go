package addressbook

import (
	"time"

	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/addressbook"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// EntryToProto 将 realmif.MemberEntry 转换为 proto 消息
func EntryToProto(entry realmif.MemberEntry) *pb.MemberEntry {
	// 转换地址
	addrs := make([][]byte, len(entry.DirectAddrs))
	for i, addr := range entry.DirectAddrs {
		if addr != nil {
			addrs[i] = addr.Bytes()
		}
	}

	return &pb.MemberEntry{
		NodeId:       entry.NodeID.Bytes(),
		Addrs:        addrs,
		NatType:      natTypeToProto(entry.NATType),
		Capabilities: entry.Capabilities,
		Online:       entry.Online,
		LastSeen:     entry.LastSeen.Unix(),
		LastUpdate:   entry.LastUpdate.Unix(),
	}
}

// EntryFromProto 从 proto 消息转换为 realmif.MemberEntry
func EntryFromProto(pb *pb.MemberEntry) (realmif.MemberEntry, error) {
	if pb == nil {
		return realmif.MemberEntry{}, ErrInvalidEntry
	}

	// 转换 NodeID
	nodeID := types.PeerID(pb.NodeId)

	// 转换地址
	addrs := make([]types.Multiaddr, 0, len(pb.Addrs))
	for _, addrBytes := range pb.Addrs {
		if len(addrBytes) > 0 {
			addr, err := types.NewMultiaddrBytes(addrBytes)
			if err != nil {
				continue // 跳过无效地址
			}
			addrs = append(addrs, addr)
		}
	}

	return realmif.MemberEntry{
		NodeID:       nodeID,
		DirectAddrs:  addrs,
		NATType:      natTypeFromProto(pb.NatType),
		Capabilities: pb.Capabilities,
		Online:       pb.Online,
		LastSeen:     time.Unix(pb.LastSeen, 0),
		LastUpdate:   time.Unix(pb.LastUpdate, 0),
	}, nil
}

// natTypeToProto 将 types.NATType 转换为 proto 枚举
func natTypeToProto(t types.NATType) pb.NATType {
	switch t {
	case types.NATTypeNone:
		return pb.NATType_NAT_TYPE_NONE
	case types.NATTypeFullCone:
		return pb.NATType_NAT_TYPE_FULL_CONE
	case types.NATTypeRestrictedCone:
		return pb.NATType_NAT_TYPE_RESTRICTED
	case types.NATTypePortRestricted:
		return pb.NATType_NAT_TYPE_PORT_RESTRICTED
	case types.NATTypeSymmetric:
		return pb.NATType_NAT_TYPE_SYMMETRIC
	default:
		return pb.NATType_NAT_TYPE_UNKNOWN
	}
}

// natTypeFromProto 从 proto 枚举转换为 types.NATType
func natTypeFromProto(t pb.NATType) types.NATType {
	switch t {
	case pb.NATType_NAT_TYPE_NONE:
		return types.NATTypeNone
	case pb.NATType_NAT_TYPE_FULL_CONE:
		return types.NATTypeFullCone
	case pb.NATType_NAT_TYPE_RESTRICTED:
		return types.NATTypeRestrictedCone
	case pb.NATType_NAT_TYPE_PORT_RESTRICTED:
		return types.NATTypePortRestricted
	case pb.NATType_NAT_TYPE_SYMMETRIC:
		return types.NATTypeSymmetric
	default:
		return types.NATTypeUnknown
	}
}

// RegisterToEntry 从 AddressRegister 消息创建 MemberEntry
func RegisterToEntry(reg *pb.AddressRegister) (realmif.MemberEntry, error) {
	if reg == nil {
		return realmif.MemberEntry{}, ErrInvalidEntry
	}

	// 转换 NodeID
	nodeID := types.PeerID(reg.NodeId)
	if nodeID.IsEmpty() {
		return realmif.MemberEntry{}, ErrInvalidNodeID
	}

	// 转换地址
	addrs := make([]types.Multiaddr, 0, len(reg.Addrs))
	for _, addrBytes := range reg.Addrs {
		if len(addrBytes) > 0 {
			addr, err := types.NewMultiaddrBytes(addrBytes)
			if err != nil {
				continue
			}
			addrs = append(addrs, addr)
		}
	}

	now := time.Now()
	return realmif.MemberEntry{
		NodeID:       nodeID,
		DirectAddrs:  addrs,
		NATType:      natTypeFromProto(reg.NatType),
		Capabilities: reg.Capabilities,
		Online:       true,
		LastSeen:     now,
		LastUpdate:   now,
	}, nil
}

// EntryToResponse 将 MemberEntry 转换为查询响应
func EntryToResponse(entry realmif.MemberEntry, found bool) *pb.AddressResponse {
	if !found {
		return &pb.AddressResponse{
			Found: false,
			Entry: nil,
		}
	}

	return &pb.AddressResponse{
		Found: true,
		Entry: EntryToProto(entry),
	}
}
