package updpackage

import (
	"net"
	"testing"
)

func Test_udp(t *testing.T) {

	src := &net.UDPAddr{
		IP:   []byte{0x1, 0x2, 0x3, 0x4},
		Port: 2384,
	}

	dst := &net.UDPAddr{
		IP:   []byte{0x7, 0x8, 0x9, 0x10, 0x11, 0x12},
		Port: 2344,
	}

	full, err := PackUDPData(dst, src, []byte("zhangweihuadidhagadkfh"))
	if err != nil {
		t.Log(" errr", err)
	}

	dstI, srcI, payload, err := UnpackUDPData(full)
	if err != nil {
		t.Log("UnpackUDPData error:", err)
		return
	}

	t.Log(dstI.IP, srcI.IP, dstI.Port, srcI.Port, string(payload))

}
