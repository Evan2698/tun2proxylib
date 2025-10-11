package buffer

import "sync"

const maxBufferSize = 4096

var pool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, maxBufferSize)
		return &b
	},
}

func Get() []byte {
	return *(pool.Get().(*[]byte))
}

func Put(b []byte) {
	pool.Put(&b)
}
