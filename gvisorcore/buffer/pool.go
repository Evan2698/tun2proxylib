package buffer

import "sync"

const Page = 1024
const TriplePage = 3 * Page
const QuadruplePage = 4 * Page

var pool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, QuadruplePage)
		return &b
	},
}

func Get() []byte {
	return *(pool.Get().(*[]byte))
}

func Put(b []byte) {
	pool.Put(&b)
}
