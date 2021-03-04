package editor

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"unicode"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/prodhe/poe/gapbuffer"
)

const (
	BufferFile uint8 = iota
	BufferDir
)

// Buffer is a buffer for editing. It uses an underlying gap buffer for storage and manages all things text related, like insert, delete, selection, searching and undo/redo.
//
// Although the underlying buffer is a pure byte slice, Buffer only works with runes and UTF-8.
type Buffer struct {
	buf      *gapbuffer.Buffer
	file     *file
	what     uint8
	dirty    bool
	q0, q1   int     // dot/cursor
	off      int     // offset for reading runes in buffer
	lastRune rune    // save the last read rune
	runeBuf  []byte  // temp buf to read rune at a time from gap buffer
	history  History // undo/redo stack
}

// initBuffer initialized a nil buffer into the zero value of buffer.
func (b *Buffer) initBuffer() {
	if b.buf == nil {
		b.buf = &gapbuffer.Buffer{}
	}
}

// NewFile sets a filename for the buffer.
func (b *Buffer) NewFile(fn string) {
	b.file = &file{name: fn}
}

// ReadFile reads content of the buffer's filename into the buffer.
func (b *Buffer) ReadFile() error {
	b.initBuffer()

	if b.file == nil || b.file.read {
		return nil // silent
	}

	info, err := os.Stat(b.file.name)
	if err != nil {
		// if the file exists, print why we could not open it
		// otherwise just close silently
		if os.IsExist(err) {
			return fmt.Errorf("%s", err)
		}
		return err
	}

	// name is a directory; list it's content into the buffer
	if info.IsDir() {
		files, err := ioutil.ReadDir(b.file.name)
		if err != nil {
			return fmt.Errorf("%s", err)
		}

		b.what = BufferDir

		// list files in dir
		for _, f := range files {
			dirchar := ""
			if f.IsDir() {
				dirchar = string(filepath.Separator)
			}
			fmt.Fprintf(b.buf, "%s%s\n", f.Name(), dirchar)
		}
		return nil
	}

	// name is a file
	fh, err := os.OpenFile(b.file.name, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("%s", err)
	}
	defer fh.Close()

	if _, err := io.Copy(b.buf, fh); err != nil {
		return fmt.Errorf("%s", err)
	}
	fh.Seek(0, 0)

	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		return fmt.Errorf("%s", err)
	}
	b.file.sha256 = fmt.Sprintf("%x", h.Sum(nil))

	b.file.mtime = info.ModTime()
	b.file.read = true

	b.what = BufferFile

	return nil
}

// IsDir returns true if the type of the this buffer is a directory listing.
func (b *Buffer) IsDir() bool {
	return b.what == BufferDir
}

// SaveFile writes content of buffer to its filename.
func (b *Buffer) SaveFile() (int, error) {
	b.initBuffer()

	if b.file == nil || b.file.name == "" {
		return 0, errors.New("no filename")
	}

	if b.what != BufferFile { // can only save file buffers
		return 0, nil
	}

	// check for file existence if we recently changed the file name
	//	openmasks := os.O_RDWR | os.O_CREATE
	//	var namechange bool
	//	if win.Name() != win.NameTag() { // user has changed name
	//		openmasks |= os.O_EXCL // must not already exist
	//		namechange = true      // to skip sha256 checksum
	//	}

	f, err := os.OpenFile(b.file.name, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		if os.IsExist(err) {
			return 0, fmt.Errorf("%s already exists", b.file.name)
		}
		return 0, err
	}
	defer f.Close()

	//h := sha256.New()
	//if _, err := io.Copy(h, f); err != nil {
	//	return 0, errors.Wrap(err, "sha256")
	//}
	//hhex := fmt.Sprintf("%x", h.Sum(nil))

	// verify checksum if the file is not newly created via a namechange
	//	if !namechange && hhex != win.file.sha256 {
	//		return 0, errors.Errorf("file has been modified outside of poe")
	//	}

	n, err := f.WriteAt(b.buf.Bytes(), 0)
	if err != nil {
		return 0, err
	}
	f.Truncate(int64(n))
	f.Sync()

	b.file.sha256 = fmt.Sprintf("%x", sha256.Sum256(b.buf.Bytes()))

	info, err := f.Stat()
	if err != nil {
		return n, err
	}
	b.file.mtime = info.ModTime()

	b.dirty = false

	return n, nil
}

// Name returns either the file from disk name or empty string if the buffer has no disk counterpart.
func (b *Buffer) Name() string {
	if b.file == nil || b.file.name == "" {
		return ""
	}
	s, _ := filepath.Abs(b.file.name)
	return s
}

// WorkDir returns the working directory of the underlying file, ie the absolute path to the file with the last part stripped. If file is a directory, its name is returned as is.
func (b *Buffer) WorkDir() string {
	switch b.what {
	case BufferFile:
		return filepath.Dir(b.Name())
	case BufferDir:
		return b.Name()
	default:
		return ""
	}
}

// Write implements io.Writer, with the side effect of storing written data into a history stack for undo/redo.
//
// If dot has content, it will be replaced by an initial deletion before inserting the bytes.
func (b *Buffer) Write(p []byte) (int, error) {
	b.initBuffer()

	// handle replace
	if len(b.ReadDot()) > 0 {
		b.Delete()
	}

	// do the actual insertion
	c := Change{b.q0, HInsert, p}
	n, err := b.commit(c)
	if err != nil {
		return n, err
	}
	b.history.Do(c)
	b.SeekDot(n, 1) // move dot
	if b.what == BufferFile {
		b.dirty = true
	}
	return n, nil
}

// Delete removes current selection in dot. If dot is empty, it selects the previous rune and deletes that.
func (b *Buffer) Delete() (int, error) {
	b.initBuffer()

	if len(b.ReadDot()) == 0 {
		b.q0--
		c, _ := b.buf.ByteAt(b.q0)
		for !utf8.RuneStart(c) {
			b.q0--
			c, _ = b.buf.ByteAt(b.q0)
		}
		if b.q0 < 0 {
			return 0, nil
		}
	}
	c := Change{b.q0, HDelete, []byte(b.ReadDot())}
	n, err := b.commit(c)
	if err != nil {
		return n, err
	}
	b.history.Do(c)
	if b.what == BufferFile {
		b.dirty = true
	}
	return n, nil
}

// Destroy will mark the buffer as completely empty and reset to 0.
func (b *Buffer) Destroy() {
	b.buf.Destroy()
	b.SetDot(0, 0)
	b.dirty = false
	b.file.read = false
}

// Len returns the number of bytes in buffer.
func (b *Buffer) Len() int {
	b.initBuffer()

	return b.buf.Len()
}

// String returns the entire text buffer as a string.
func (b *Buffer) String() string {
	b.initBuffer()

	return string(b.buf.Bytes())
}

// Dirty returns true if the buffer has changed since last save.
func (b *Buffer) Dirty() bool {
	return b.dirty
}

// ReadRune reads a rune from buffer and advances the internal offset. This could be called in sequence to get all runes from buffer. This populates LastRune().
func (b *Buffer) ReadRune() (r rune, size int, err error) {
	r, size, err = b.ReadRuneAt(b.off)
	b.off += size
	b.lastRune = r
	return
}

// UnreadRune returns the rune before the current Seek offset and moves the offset to point to that. This could be called in sequence to scan backwards.
func (b *Buffer) UnreadRune() (r rune, size int, err error) {
	b.off--
	r, size, err = b.ReadRuneAt(b.off)
	b.off++
	if err != nil {
		return
	}
	b.off -= size
	return
}

// ReadRuneAt returns the rune and its size at offset. If the given offset (in byte count) is not a valid rune, it will try to back up until it finds a valid starting point for a rune and return that one.
//
// This is basically a Seek(offset) followed by a ReadRune(), but does not affect the internal offset for future reads.
func (b *Buffer) ReadRuneAt(offset int) (r rune, size int, err error) {
	b.initBuffer()

	var c byte
	c, err = b.buf.ByteAt(offset)
	if err != nil {
		return 0, 0, err
	}
	for !utf8.RuneStart(c) {
		offset--
		c, err = b.buf.ByteAt(offset)
		if err != nil {
			return 0, 0, err
		}
	}

	if c < utf8.RuneSelf {
		return rune(c), 1, nil
	}

	if cap(b.runeBuf) < 4 {
		b.runeBuf = make([]byte, 4) // max length of a rune
	}
	_, err = b.buf.ReadAt(b.runeBuf, offset)
	if err != nil {
		return 0, 0, err
	}
	r, n := utf8.DecodeRune(b.runeBuf)

	return r, n, nil
}

// LastRune returns the last rune read by ReadRune().
func (b *Buffer) LastRune() rune {
	return b.lastRune
}

// ReadDot returns content of current dot.
func (b *Buffer) ReadDot() string {
	b.initBuffer()

	if b.q0 == b.q1 {
		return ""
	}
	buf := make([]byte, b.q1-b.q0)
	_, err := b.buf.ReadAt(buf, b.q0)
	if err != nil {
		return ""
	}
	return string(buf)
}

// Dot returns current offsets for dot.
func (b *Buffer) Dot() (int, int) {
	return b.q0, b.q1
}

// Seek implements io.Seeker and sets the internal offset for next ReadRune() or UnreadRune(). If the offset is not a valid rune start, it will backup until it finds one.
func (b *Buffer) Seek(offset, whence int) (int, error) {
	b.initBuffer()

	b.off = offset

	switch whence {
	case io.SeekStart:
		b.off = offset
	case io.SeekCurrent:
		b.off += offset
	case io.SeekEnd:
		b.off = b.Len() + offset
	default:
		return 0, errors.New("invalid whence")
	}

	c, _ := b.buf.ByteAt(b.off)
	for !utf8.RuneStart(c) {
		b.off--
		c, _ = b.buf.ByteAt(b.off)
	}

	return b.off, nil
}

// SeekDot sets the dot to a single offset in the text buffer.
func (b *Buffer) SeekDot(offset, whence int) (int, error) {
	switch whence {
	case io.SeekStart:
		q0, _, err := b.SetDot(offset, offset)
		return q0, err
	case io.SeekCurrent:
		q0, _, err := b.SetDot(b.q0+offset, b.q0+offset)
		return q0, err
	case io.SeekEnd:
		q0, _, err := b.SetDot(b.Len()+offset, b.Len()+offset)
		return q0, err
	default:
		return 0, errors.New("invalid whence")
	}
}

// SetDot sets both ends of the dot into an absolute position. It will check the given offsets and adjust them accordingly, so they are not out of bounds or on an invalid rune start. It returns the final offsets. Error is always nil.
func (b *Buffer) SetDot(q0, q1 int) (int, int, error) {
	b.initBuffer()

	b.q0, b.q1 = q0, q1

	// check out of bounds
	if b.q0 < 0 {
		b.q0 = 0
	}
	if b.q1 < 0 {
		b.q1 = 0
	}
	if b.q0 >= b.buf.Len() {
		b.q0 = b.buf.Len()
	}
	if b.q1 >= b.buf.Len() {
		b.q1 = b.buf.Len()
	}

	// q0 must never be greater than q1
	if b.q0 > b.q1 {
		b.q0 = b.q1
	}
	if b.q1 < b.q0 {
		b.q1 = b.q0
	}

	// set only to valid rune start
	var c byte
	c, _ = b.buf.ByteAt(b.q0)
	for !utf8.RuneStart(c) {
		b.q0--
		c, _ = b.buf.ByteAt(b.q0)
	}
	c, _ = b.buf.ByteAt(b.q1)
	for !utf8.RuneStart(c) {
		b.q1--
		c, _ = b.buf.ByteAt(b.q1)
	}

	return b.q0, b.q1, nil
}

// ExpandDot expands the current selection in positive or negative offset. A positive offset expands forwards and a negative expands backwards. Q is 0 or 1, either the left or the right end of the dot.
func (b *Buffer) ExpandDot(q, offset int) {
	if q < 0 || q > 1 {
		return
	}

	if q == 0 {
		b.SetDot(b.q0+offset, b.q1)
	} else {
		b.SetDot(b.q0, b.q1+offset)
	}
}

// Select expands current dot into something useful.
//
// If given pos is adjacent to a quote, parenthese, curly brace or bracket, it tries to select into the matching pair.
//
// If on newline, select the whole line.
//
// Otherwise, select word (longest alphanumeric sequence).
func (b *Buffer) Select(offset int) {
	offset, _ = b.Seek(offset, io.SeekStart)
	start, end := offset, offset

	// space
	//start -= t.PrevSpace(start)
	//end += t.NextSpace(end)

	// word
	start -= b.PrevWord(start)
	end += b.NextWord(end)

	// return a single char selection if no word was found
	if start == end {
		b.Seek(offset, io.SeekStart)
		_, size, _ := b.ReadRune()
		end += size
	}

	// Set dot
	b.SetDot(start, end)
}

func (b *Buffer) NextSpace(offset int) (n int) {
	offset, _ = b.Seek(offset, io.SeekStart)

	r, size, err := b.ReadRune()
	if err != nil {
		return 0
	}
	for !unicode.IsSpace(r) {
		n += size
		r, size, err = b.ReadRune()
		if err != nil {
			if err == io.EOF {
				return n
			}
			return 0
		}
	}

	return n
}

func (b *Buffer) PrevSpace(offset int) (n int) {
	offset, _ = b.Seek(offset, io.SeekStart)

	r, size, err := b.ReadRuneAt(offset)
	if err != nil {
		return 0
	}
	for !unicode.IsSpace(r) {
		r, size, err = b.UnreadRune()
		if err != nil {
			if err == gapbuffer.ErrOutOfRange {
				return n
			}
		}
		n += size
	}

	if n > 0 {
		n -= size // remove last iteration
	}

	return n
}

func (b *Buffer) NextWord(offset int) (n int) {
	offset, _ = b.Seek(offset, io.SeekStart)

	r, size, err := b.ReadRune()
	if err != nil {
		return 0
	}
	for unicode.IsLetter(r) || unicode.IsDigit(r) {
		n += size
		r, size, err = b.ReadRune()
		if err != nil {
			if err == io.EOF {
				return n
			}
			return 0
		}
	}

	return n
}

func (b *Buffer) PrevWord(offset int) (n int) {
	offset, _ = b.Seek(offset, io.SeekStart)

	r, size, _ := b.ReadRuneAt(offset)
	for unicode.IsLetter(r) || unicode.IsDigit(r) {
		r, size, _ = b.UnreadRune()
		n += size
	}

	if n > 0 {
		n -= size // remove last iteration
	}

	return n
}

// NextDelim returns number of bytes from given offset up until next delimiter.
func (b *Buffer) NextDelim(delim rune, offset int) (n int) {
	b.Seek(offset, io.SeekStart)

	r, size, err := b.ReadRune()
	if err != nil {
		return 0
	}

	for r != delim {
		n += size
		r, size, err = b.ReadRune()
		if err != nil {
			if err == io.EOF {
				return n
			}
			return 0
		}
	}

	return n
}

// PrevDelim returns number of bytes from given offset up until next delimiter.
func (b *Buffer) PrevDelim(delim rune, offset int) (n int) {
	b.Seek(offset, io.SeekStart)
	r, size, err := b.UnreadRune()
	if err != nil {
		return 0
	}
	n += size

	for r != delim {
		r, size, err = b.UnreadRune()
		n += size
		if err != nil {
			if err == gapbuffer.ErrOutOfRange {
				return n
			}
			return 0
		}
	}

	return n
}

func (b *Buffer) Undo() error {
	c, err := b.history.Undo()
	if err != nil {
		return errors.Wrap(err, "undo")
	}
	b.commit(c)

	// highlight text
	b.SetDot(c.offset, c.offset+len(c.content))

	return nil
}

func (b *Buffer) Redo() error {
	c, err := b.history.Redo()
	if err != nil {
		return errors.Wrap(err, "redo")
	}
	b.commit(c)

	if c.action == HDelete {
		b.SetDot(c.offset, c.offset)
	} else {
		b.SetDot(c.offset+len(c.content), c.offset+len(c.content))
	}
	return nil
}

func (b *Buffer) commit(c Change) (int, error) {
	b.initBuffer()

	switch c.action {
	case HInsert:
		b.buf.Seek(c.offset) // sync gap buffer
		n, err := b.buf.Write([]byte(c.content))
		if err != nil {
			return 0, err
		}
		return n, err
	case HDelete:
		n := len(c.content)
		b.buf.Seek(c.offset + n) // sync gap buffer
		for i := n; i > 0; i-- {
			b.buf.Delete() // gap buffer deletes one byte at a time
		}
		return n, nil
	default:
		return 0, errors.New("invalid action in change")
	}
}

/* History */
type HistoryAction uint8

const (
	HInsert HistoryAction = iota
	HDelete
)

type Change struct {
	offset  int
	action  HistoryAction
	content []byte
}

type ChangeSet []*Change

type History struct {
	done   []Change
	recall []Change
}

func (h *History) Do(c Change) {
	h.done = append(h.done, c)
	h.recall = nil // clear old recall stack on new do
}

func (h *History) Undo() (Change, error) {
	if len(h.done) == 0 {
		return Change{}, errors.New("no history")
	}
	lastdone := h.done[len(h.done)-1]
	h.recall = append(h.recall, lastdone)
	h.done = h.done[:len(h.done)-1] // remove last one

	// Reverse the done action so the returned change can be applied directly.
	switch lastdone.action {
	case HInsert:
		lastdone.action = HDelete
	case HDelete:
		lastdone.action = HInsert
	}

	return lastdone, nil
}

func (h *History) Redo() (Change, error) {
	if len(h.recall) == 0 {
		return Change{}, errors.New("no recall history")
	}
	lastrecall := h.recall[len(h.recall)-1]
	h.done = append(h.done, lastrecall)
	h.recall = h.recall[:len(h.recall)-1] //remove last one
	return lastrecall, nil
}
