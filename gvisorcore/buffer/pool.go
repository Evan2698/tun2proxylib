package buffer

import "sync"

const (
	Page          = 1024
	TriplePage    = 3 * Page
	QuadruplePage = 4 * Page
)

var pool = sync.Pool{
	New: func() any {
		return make([]byte, QuadruplePage)
	},
}

func Get() []byte {
	return pool.Get().([]byte)
}

func Put(b []byte) {
	if cap(b) != QuadruplePage {
		return
	}

	pool.Put(b[:QuadruplePage])
}
