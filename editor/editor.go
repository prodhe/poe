package editor

import (
	"path/filepath"
	"time"

	"github.com/prodhe/poe/gapbuffer"
)

// Editor is the edit component that holds text buffers. A UI of some sort operates on the editor to manipulate buffers.
type Editor interface {
	NewBuffer() (id int64, buf *Buffer)
	Buffer(id int64) *Buffer
	Buffers() ([]int64, []*Buffer)
	Current() *Buffer
	LoadBuffers(filenames []string)
	CloseBuffer(id int64)
	WorkDir() string
	Len() int
	Run(cmd string)
}

// New returns an empty editor with no buffers loaded.
func New() Editor {
	e := &editor{}
	e.buffers = map[int64]*Buffer{}
	e.workdir, _ = filepath.Abs(".")
	e.initCommands()
	return e
}

// editor implements Editor.
type editor struct {
	buffers map[int64]*Buffer
	current int64 // id ref to current buffer
	workdir string
}

// NewBuffer creates an empty buffer and appends it to the editor. Returns the new id and the new buffer.
func (e *editor) NewBuffer() (id int64, buf *Buffer) {
	buf = &Buffer{buf: &gapbuffer.Buffer{}}
	id = e.genBufferID()
	e.buffers[id] = buf
	e.current = id
	return id, buf
}

// Buffer returns the buffer with given index. Nil if id not found.
func (e *editor) Buffer(id int64) *Buffer {
	if _, ok := e.buffers[id]; ok {
		e.current = id
	}
	return e.buffers[id]
}

// Buffers returns a slice of IDs and a slice of buffers.
func (e *editor) Buffers() ([]int64, []*Buffer) {
	ids := make([]int64, 0, len(e.buffers))
	bs := make([]*Buffer, 0, len(e.buffers))
	for i, b := range e.buffers {
		ids = append(ids, i)
		bs = append(bs, b)
	}
	return ids, bs
}

// Current returns the current buffer.
func (e *editor) Current() *Buffer {
	return e.buffers[e.current]
}

// CloseBuffer deletes the given buffer from memory. No warnings. Here be dragons.
func (e *editor) CloseBuffer(id int64) {
	delete(e.buffers, id)
}

// Len returns number of buffers currently in the editor.
func (e *editor) Len() int {
	return len(e.buffers)
}

// WorkDir returns the base working directory of the editor.
func (e *editor) WorkDir() string {
	if e.workdir == "" {
		d, _ := filepath.Abs(".")
		return d
	}
	return e.workdir
}

// LoadBuffers reads files from disk and loads them into windows. Screen need to be initialized.
func (e *editor) LoadBuffers(fns []string) {
	// load given filenames and append to buffer list
	for _, fn := range fns {
		_, buf := e.NewBuffer()
		buf.NewFile(fn)
		buf.ReadFile()
	}

	if len(fns) == 0 {
		e.NewBuffer()
	}
}

func (e *editor) genBufferID() int64 {
	return time.Now().UnixNano()
}
