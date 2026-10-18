package pkg

import (
	"net/url"
	"testing"
)

func TestParseVsockAddress(t *testing.T) {
	cases := []struct {
		listen string
		cid    uint32
		port   uint32
		fail   bool
	}{
		{"vsock://any:5000", vsockCIDAny, 5000, false},
		{"vsock://any", vsockCIDAny, defaultVsockPort, false},
		{"vsock://host:1234", vsockCIDHost, 1234, false},
		{"vsock://hypervisor:7", vsockCIDHypervisor, 7, false},
		{"vsock://42:80", 42, 80, false},
		{"vsock://nope:80", 0, 0, true},
		{"vsock://any:notaport", 0, 0, true},
		{"vsock://any:-1", 0, 0, true},
	}

	for _, c := range cases {
		u, err := url.Parse(c.listen)

		if err != nil {
			// net/url already refuses a port that isn't a number
			if !c.fail {
				t.Fatalf("%s: %v", c.listen, err)
			}

			continue
		}

		cid, port, err := parseVsockAddress(u)

		if c.fail {
			if err == nil {
				t.Errorf("%s: expected an error, got cid=%d port=%d", c.listen, cid, port)
			}

			continue
		}

		if err != nil || cid != c.cid || port != c.port {
			t.Errorf("%s: got cid=%d port=%d err=%v, want cid=%d port=%d", c.listen, cid, port, err, c.cid, c.port)
		}
	}
}
