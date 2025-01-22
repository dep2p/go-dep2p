package nat

import (
	"net"

	"github.com/dep2p/go-dep2p/p2plib/netroute"
)

func getDefaultGateway() (net.IP, error) {
	router, err := netroute.New()
	if err != nil {
		return nil, err
	}

	_, ip, _, err := router.Route(net.IPv4zero)
	return ip, err
}
