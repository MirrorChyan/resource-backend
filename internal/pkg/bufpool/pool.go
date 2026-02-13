package bufpool

import "sync"

const blockSize = 64 * 1024

var bufferPool = sync.Pool{
	New: func() any {
		val := make([]byte, blockSize)
		return &val
	},
}

func GetBuffer() *[]byte {
	return bufferPool.Get().(*[]byte)
}

func PutBuffer(b *[]byte) {
	bufferPool.Put(b)
}
