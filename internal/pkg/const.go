package pkg

import "sync"

const blockSize = 4 * 1024 * 1024

var bufferPool = sync.Pool{
	New: func() any {
		return make([]byte, blockSize)
	},
}

func GetBuffer() []byte {
	return bufferPool.Get().([]byte)
}

func PutBuffer(b []byte) {
	bufferPool.Put(b)
}
