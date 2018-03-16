package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/prodhe/poe/gapbuffer"
)

// Editor is the edit component that holds text buffers. A UI of some sort operates on the editor to manipulate buffers.
type Editor interface {
	NewBuffer() (id int64, buf *Buffer)
	Buffer(id int64) *Buffer
	Buffers() ([]int64, []*Buffer)
	LoadBuffers(filenames []string)
	CloseBuffer(id int64)
	WorkDir() string
	Len() int
	Edit(bufid int64, cmd string) string
}

// New returns an empty editor with no buffers loaded.
func New() Editor {
	e := &ed{}
	e.buffers = map[int64]*Buffer{}
	e.workdir, _ = filepath.Abs(".")
	return e
}

// ed implements Editor.
type ed struct {
	buffers map[int64]*Buffer
	workdir string
}

// NewBuffer creates an empty buffer and appends it to the editor. Returns the new id and the new buffer.
func (e *ed) NewBuffer() (id int64, buf *Buffer) {
	buf = &Buffer{buf: &gapbuffer.Buffer{}}
	id = e.genBufferID()
	e.buffers[id] = buf
	return id, buf
}

// Buffer returns the buffer with given index. Nil if id not found.
func (e *ed) Buffer(id int64) *Buffer {
	if _, ok := e.buffers[id]; ok {
		return e.buffers[id]
	}
	return nil
}

// Buffers returns a slice of IDs and a slice of buffers.
func (e *ed) Buffers() ([]int64, []*Buffer) {
	ids := make([]int64, 0, len(e.buffers))
	bs := make([]*Buffer, 0, len(e.buffers))
	for i, b := range e.buffers {
		ids = append(ids, i)
		bs = append(bs, b)
	}
	return ids, bs
}

// CloseBuffer deletes the given buffer from memory. No warnings. Here be dragons.
func (e *ed) CloseBuffer(id int64) {
	delete(e.buffers, id)
}

// Len returns number of buffers currently in the editor.
func (e *ed) Len() int {
	return len(e.buffers)
}

// WorkDir returns the base working directory of the editor.
func (e *ed) WorkDir() string {
	if e.workdir == "" {
		d, _ := filepath.Abs(".")
		return d
	}
	return e.workdir
}

// LoadBuffers reads files from disk and loads them into windows. Screen need to be initialized.
func (e *ed) LoadBuffers(fns []string) {
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

func (e *ed) genBufferID() int64 {
	return time.Now().UnixNano()
}

func (e *ed) Edit(bufid int64, args string) string {
	if len(args) < 1 {
		return ""
	}

	if _, ok := e.buffers[bufid]; !ok {
		// no such bufid
		return ""
	}

	switch args[0] {
	case 'f':
		var names []string
		for _, buf := range e.buffers {
			names = append(names, fmt.Sprintf("%s", buf.Name()))
		}
		return fmt.Sprintf("buffers:\n%s", strings.Join(names, "\n"))
	case '!':
		os.Chdir(e.buffers[bufid].WorkDir())
		cmd := strings.Split(string(args[1:]), " ")
		path, err := exec.LookPath(cmd[0])
		if err != nil { // path not found or not executable
			//return fmt.Sprintf("cannot execute: %s", cmd[0])
			return ""
		}
		out, err := exec.Command(path, cmd[1:]...).Output()
		if err != nil {
			return fmt.Sprintf("error: %s", err)
			break
		}
		outstr := string(out)
		return outstr
	}

	// no match
	return "?"
}
