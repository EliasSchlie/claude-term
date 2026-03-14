package terminal

const maxBufferSize = 100 * 1024 // 100KB

// Buffer manages terminal output with a max size limit.
// Not thread-safe — caller must synchronize.
type Buffer struct {
	data []byte
}

// NewBuffer creates an empty buffer.
func NewBuffer() *Buffer {
	return &Buffer{}
}

// Write appends data to the buffer, trimming from the front if needed.
func (b *Buffer) Write(p []byte) {
	b.data = append(b.data, p...)
	if len(b.data) > maxBufferSize {
		b.data = b.data[len(b.data)-maxBufferSize:]
		// Skip UTF-8 continuation bytes at the start to avoid mid-character splits.
		for len(b.data) > 0 && b.data[0]&0xC0 == 0x80 {
			b.data = b.data[1:]
		}
	}
}

// Bytes returns a copy of the buffered data.
func (b *Buffer) Bytes() []byte {
	out := make([]byte, len(b.data))
	copy(out, b.data)
	return out
}

// Len returns the current buffer size.
func (b *Buffer) Len() int {
	return len(b.data)
}
