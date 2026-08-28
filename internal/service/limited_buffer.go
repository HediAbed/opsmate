package service

import "sync"

const initialLimitedBufferCapacity = 4 * 1024

type limitedBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{data: make([]byte, 0, min(limit, initialLimitedBufferCapacity)), limit: limit}
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	written := len(data)
	remaining := b.limit - len(b.data)
	if remaining <= 0 {
		b.truncated = b.truncated || written > 0
		return written, nil
	}
	if len(data) > remaining {
		b.data = append(b.data, data[:remaining]...)
		b.truncated = true
		return written, nil
	}
	b.data = append(b.data, data...)
	return written, nil
}

func (b *limitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}

func (b *limitedBuffer) String() string {
	return string(b.Bytes())
}

func (b *limitedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
