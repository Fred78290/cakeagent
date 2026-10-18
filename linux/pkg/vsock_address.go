package pkg

import (
	"fmt"
	"net/url"
	"strconv"
)

const (
	// Well known vsock context ids (VMADDR_CID_*).
	vsockCIDHypervisor uint32 = 0
	vsockCIDHost       uint32 = 2
	vsockCIDAny        uint32 = 0xFFFFFFFF

	defaultVsockPort = 5000
)

// parseVsockAddress reads a vsock://<cid|any|host|hypervisor>[:port] listen address, the same syntax on every platform.
func parseVsockAddress(u *url.URL) (cid uint32, port uint32, err error) {
	port = defaultVsockPort

	if u.Port() != "" {
		var p int

		if p, err = strconv.Atoi(u.Port()); err != nil || p < 0 || p > 0xFFFFFFFE {
			return 0, 0, fmt.Errorf("invalid port: %q", u.Port())
		}

		port = uint32(p)
	}

	switch hostname := u.Hostname(); hostname {
	case "any":
		cid = vsockCIDAny
	case "host":
		cid = vsockCIDHost
	case "hypervisor":
		cid = vsockCIDHypervisor
	default:
		var c uint64

		if c, err = strconv.ParseUint(hostname, 10, 32); err != nil {
			return 0, 0, fmt.Errorf("invalid cid: %q", hostname)
		}

		cid = uint32(c)
	}

	return
}
