package gapbuffer

import (
	"errors"
	"io"
)

// ErrOutOfRange is returned when given position is out of range for the buffer.
var ErrOutOfRange = errors.New("index out of range")

type Buffer struct {
	buf       []byte
	bootstrap [64]byte
	start     int // gap start, is considered empty
	end       int // gap end, holds next byte counting from before the gap
}

func (b *Buffer) Bytes() []byte {
	buf := make([]byte, b.Len())
	b.ReadAt(buf, 0)
	return buf
}

// Destroy will erase the Buffer by zeroising all fields.
func (b *Buffer) Destroy() {
	b.start = 0
	b.end = len(b.buf)
}

// Byte returns current byte right after the gap. If the gap is at the end, the return will be 0.
func (b *Buffer) Byte() byte {
	if b.end >= len(b.buf) {
		return 0 // EOF
	}

	return b.buf[b.end]
}

// ByteAt returns the byte at the given offset, ignoring and hiding the gap.
func (b *Buffer) ByteAt(offset int) (byte, error) {
	if offset >= b.start {
		offset += b.gapLen()
	}

	if offset < 0 {
		return 0, ErrOutOfRange
	}
	if offset >= len(b.buf) {
		return 0, io.EOF
	}

	return b.buf[offset], nil
}

// gapLen returns the length of the gap.
func (b *Buffer) gapLen() int {
	return b.end - b.start
}

// Len returns the length of actual data.
func (b *Buffer) Len() int {
	return len(b.buf) - b.gapLen()
}

// Cap returns the capacity of the Buffer, including the gap.
func (b *Buffer) Cap() int {
	return cap(b.buf)
}

// Pos returns the current start position of the gap. This is where next write will appear.
func (b *Buffer) Pos() int {
	return b.start
}

// Seek is moving the gap to a given offset from origin.
func (b *Buffer) Seek(newpos int) {
	// out of range
	if newpos < 0 {
		//b.Seek(0)
		panic("index below zero")
		return
	}
	if newpos > b.Len() {
		b.Seek(b.Len())
		return
	}

	if newpos < b.start { // move backwards
		for b.start > newpos {
			b.backward()
		}
	} else if newpos > b.start { // move forward
		for b.start < newpos {
			b.forward()
		}
	}
}

// Forward is moving the gap one byte forward.
func (b *Buffer) forward() {
	if b.end >= len(b.buf) {
		return
	}
	b.buf[b.start] = b.buf[b.end]
	b.start++
	b.end++
}

// Backward is moving the gap one byte backwards.
func (b *Buffer) backward() {
	if b.start <= 0 {
		return
	}
	b.buf[b.end-1] = b.buf[b.start-1]
	b.start--
	b.end--
}

// Delete will expand the gap by 1, deleting the character before the gap. Returns the byte that was deleted.
func (b *Buffer) Delete() byte {
	if b.start-1 < 0 {
		return 0
	}
	b.start--
	return b.buf[b.start]
}

// grow will grow the buffer if necessary.
func (b *Buffer) grow() {
	if b.buf == nil {
		b.buf = b.bootstrap[:]
		fill := make([]byte, cap(b.buf))
		b.buf = append(b.buf, fill...)

		b.start = 0
		b.end = cap(b.buf)
		return
	}
	oldpos := b.start
	b.Seek(cap(b.buf))

	b.buf = append(b.buf, 0)
	exp := cap(b.buf)
	b.end += exp + 1

	fill := make([]byte, exp)
	b.buf = append(b.buf, fill...)

	b.Seek(oldpos)
}

// Write writes p into the Buffer at current gap position. The Buffer will expand if needed and this is the only time any new memory allocation is done. The resizing and expanding strategy is handled by the underlying internal byte slice. Cap() will return the size of this slice.
//
// It will never return any other error than nil.
func (b *Buffer) Write(p []byte) (int, error) {
	if p == nil {
		return 0, nil
	}
	for _, c := range p {
		if b.gapLen() == 0 {
			b.grow()
		}
		b.buf[b.start] = c
		b.start++
	}
	return len(p), nil
}

// Read implements io.Reader, returning number of bytes from the Buffer while ignoring the gap.
func (b *Buffer) Read(p []byte) (int, error) {
	return b.ReadAt(p, b.end)
}

// ReadAt fills p with bytes starting at offset from the Buffer, ignoring the gap. Returns number of bytes and an error.
func (b *Buffer) ReadAt(p []byte, offset int) (n int, err error) {
	if offset < 0 {
		return 0, ErrOutOfRange
	}
	if offset >= b.Len() {
		return 0, io.EOF
	}

	for n = 0; n < len(p) && offset < b.Len(); n++ {
		c, err := b.ByteAt(offset)
		if err != nil {
			return n, err
		}

		p[n] = c
		offset++
	}

	return n, nil
}
