package core

import (
	"sync"
)

var udpConns sync.Map

type udpConnId struct {
	src string
}

var (
	UdpConMap sync.Map
	UdpOnce   sync.Once
	Udplock   sync.Mutex
	timeout        = int32(10 * 60 * 12) // 30 minutes
	Exit      bool = false
)
