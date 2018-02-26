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
	BufferScratch uint8 = iota
	BufferFile
	BufferDir
)

// Buffer is a buffer for editing. It uses an underlying gap buffer for storage and manages all things text related, like insert, delete, selection, searching and undo/redo.
//
// Although the underlying buffer is a pure byte slice, Buffer only works with runes and UTF-8.
type Buffer struct {
	buf      *gapbuffer.Buffer
	file     *File
	what     uint8
	dirty    bool
	q0, q1   int     // dot/cursor
	off      int     // offset for reading runes in buffer
	lastRune rune    // save the last read rune
	runeBuf  []byte  // temp buf to read rune at a time from gap buffer
	history  History // undo/redo stack
}

func (t *Buffer) initBuffer() {
	if t.buf == nil {
		t.buf = &gapbuffer.Buffer{}
	}
}

func (t *Buffer) NewFile(fn string) {
	t.file = &File{name: fn}
}

func (t *Buffer) ReadFile() error {
	t.initBuffer()

	if t.file == nil || t.file.read {
		return nil // silent
	}

	info, err := os.Stat(t.file.name)
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
		files, err := ioutil.ReadDir(t.file.name)
		if err != nil {
			return fmt.Errorf("%s", err)
		}

		t.what = BufferDir

		// list files in dir
		for _, f := range files {
			dirchar := ""
			if f.IsDir() {
				dirchar = string(filepath.Separator)
			}
			fmt.Fprintf(t.buf, "%s%s\n", f.Name(), dirchar)
		}
		return nil
	}

	// name is a file
	fh, err := os.OpenFile(t.file.name, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("%s", err)
	}
	defer fh.Close()

	if _, err := io.Copy(t.buf, fh); err != nil {
		return fmt.Errorf("%s", err)
	}
	fh.Seek(0, 0)

	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		return fmt.Errorf("%s", err)
	}
	t.file.sha256 = fmt.Sprintf("%x", h.Sum(nil))

	t.file.mtime = info.ModTime()
	t.file.read = true

	t.what = BufferFile

	return nil
}

func (t *Buffer) SaveFile() (int, error) {
	t.initBuffer()

	if t.file == nil || t.file.name == "" {
		return 0, errors.New("no filename")
	}

	if t.what != BufferFile { // can only save file buffers
		return 0, nil
	}

	// check for file existence if we recently changed the file name
	//	openmasks := os.O_RDWR | os.O_CREATE
	//	var namechange bool
	//	if win.Name() != win.NameTag() { // user has changed name
	//		openmasks |= os.O_EXCL // must not already exist
	//		namechange = true      // to skip sha256 checksum
	//	}

	f, err := os.OpenFile(t.file.name, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		if os.IsExist(err) {
			return 0, fmt.Errorf("%s already exists", t.file.name)
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

	n, err := f.WriteAt(t.buf.Bytes(), 0)
	if err != nil {
		return 0, err
	}
	f.Truncate(int64(n))
	f.Sync()

	t.file.sha256 = fmt.Sprintf("%x", sha256.Sum256(t.buf.Bytes()))

	info, err := f.Stat()
	if err != nil {
		return n, err
	}
	t.file.mtime = info.ModTime()

	t.dirty = false

	return n, nil
}

// Name returns either the file from disk name or empty string if the buffer has no disk counterpart.
func (t *Buffer) Name() string {
	if t.file == nil || t.file.name == "" {
		return ""
	}
	s, _ := filepath.Abs(t.file.name)
	return s
}

func (t *Buffer) WorkDir() string {
	switch t.what {
	case BufferFile, BufferScratch:
		return filepath.Dir(t.Name())
	case BufferDir:
		return t.Name()
	default:
		return ""
	}
}

// Write implements io.Writer, with the side effect of storing written data into a history stack for undo/redo.
//
// If dot has content, it will be replaced by an initial deletion before inserting the bytes.
func (t *Buffer) Write(p []byte) (int, error) {
	t.initBuffer()

	// handle replace
	if len(t.ReadDot()) > 0 {
		t.Delete()
	}

	// do the actual insertion
	c := Change{t.q0, HInsert, p}
	n, err := t.commit(c)
	if err != nil {
		return n, err
	}
	t.history.Do(c)
	t.SeekDot(n, 1) // move dot
	return n, nil
}

// Delete removes current selection in dot. If dot is empty, it selects the previous rune and deletes that.
func (t *Buffer) Delete() (int, error) {
	t.initBuffer()

	if len(t.ReadDot()) == 0 {
		t.q0--
		c, _ := t.buf.ByteAt(t.q0)
		for !utf8.RuneStart(c) {
			t.q0--
			c, _ = t.buf.ByteAt(t.q0)
		}
		if t.q0 < 0 {
			return 0, nil
		}
	}
	c := Change{t.q0, HDelete, []byte(t.ReadDot())}
	n, err := t.commit(c)
	if err != nil {
		return n, err
	}
	t.history.Do(c)
	return n, nil
}

// Len returns the number of bytes in buffer.
func (t *Buffer) Len() int {
	t.initBuffer()

	return t.buf.Len()
}

// String returns the entire text buffer as a string.
func (t *Buffer) String() string {
	t.initBuffer()

	return string(t.buf.Bytes())
}

// ReadRune reads a rune from buffer and advances the internal offset. This could be called in sequence to get all runes from buffer. This populates LastRune().
func (t *Buffer) ReadRune() (r rune, size int, err error) {
	r, size, err = t.ReadRuneAt(t.off)
	t.off += size
	t.lastRune = r
	return
}

// UnreadRune returns the rune before the current Seek offset and moves the offset to point to that. This could be called in sequence to scan backwards.
func (t *Buffer) UnreadRune() (r rune, size int, err error) {
	t.off--
	r, size, err = t.ReadRuneAt(t.off)
	t.off++
	if err != nil {
		return
	}
	t.off -= size
	return
}

// ReadRuneAt returns the rune and its size at offset. If the given offset (in byte count) is not a valid rune, it will try to back up until it finds a valid starting point for a rune and return that one.
//
// This is basically a Seek(offset) followed by a ReadRune(), but does not affect the internal offset for future reads.
func (t *Buffer) ReadRuneAt(offset int) (r rune, size int, err error) {
	t.initBuffer()

	var c byte
	c, err = t.buf.ByteAt(offset)
	if err != nil {
		return 0, 0, err
	}
	for !utf8.RuneStart(c) {
		offset--
		c, err = t.buf.ByteAt(offset)
		if err != nil {
			return 0, 0, err
		}
	}

	if c < utf8.RuneSelf {
		return rune(c), 1, nil
	}

	if cap(t.runeBuf) < 4 {
		t.runeBuf = make([]byte, 4) // max length of a rune
	}
	_, err = t.buf.ReadAt(t.runeBuf, offset)
	if err != nil {
		return 0, 0, err
	}
	r, n := utf8.DecodeRune(t.runeBuf)

	return r, n, nil
}

// LastRune returns the last rune read by ReadRune().
func (t *Buffer) LastRune() rune {
	return t.lastRune
}

// ReadDot returns content of current dot.
func (t *Buffer) ReadDot() string {
	t.initBuffer()

	if t.q0 == t.q1 {
		return ""
	}
	buf := make([]byte, t.q1-t.q0)
	_, err := t.buf.ReadAt(buf, t.q0)
	if err != nil {
		return ""
	}
	return string(buf)
}

// Dot returns current offsets for dot.
func (t *Buffer) Dot() (int, int) {
	return t.q0, t.q1
}

// Seek implements io.Seeker and sets the internal offset for next ReadRune() or UnreadRune(). If the offset is not a valid rune start, it will backup until it finds one.
func (t *Buffer) Seek(offset, whence int) (int, error) {
	t.initBuffer()

	t.off = offset

	switch whence {
	case io.SeekStart:
		t.off = offset
	case io.SeekCurrent:
		t.off += offset
	case io.SeekEnd:
		t.off = t.Len() + offset
	default:
		return 0, errors.New("invalid whence")
	}

	c, _ := t.buf.ByteAt(t.off)
	for !utf8.RuneStart(c) {
		t.off--
		c, _ = t.buf.ByteAt(t.off)
	}

	return t.off, nil
}

// SeekDot sets the dot to a single offset in the text buffer.
func (t *Buffer) SeekDot(offset, whence int) (int, error) {
	switch whence {
	case io.SeekStart:
		q0, _, err := t.SetDot(offset, offset)
		return q0, err
	case io.SeekCurrent:
		q0, _, err := t.SetDot(t.q0+offset, t.q0+offset)
		return q0, err
	case io.SeekEnd:
		q0, _, err := t.SetDot(t.Len()+offset, t.Len()+offset)
		return q0, err
	default:
		return 0, errors.New("invalid whence")
	}
}

// SetDot sets both ends of the dot into an absolute position. It will check the given offsets and adjust them accordingly, so they are not out of bounds or on an invalid rune start. It returns the final offsets. Error is always nil.
func (t *Buffer) SetDot(q0, q1 int) (int, int, error) {
	t.initBuffer()

	t.q0, t.q1 = q0, q1

	// check out of bounds
	if t.q0 < 0 {
		t.q0 = 0
	}
	if t.q1 < 0 {
		t.q1 = 0
	}
	if t.q0 >= t.buf.Len() {
		t.q0 = t.buf.Len()
	}
	if t.q1 >= t.buf.Len() {
		t.q1 = t.buf.Len()
	}

	// q0 must never be greater than q1
	if t.q0 > t.q1 {
		t.q0 = t.q1
	}
	if t.q1 < t.q0 {
		t.q1 = t.q0
	}

	// set only to valid rune start
	var c byte
	c, _ = t.buf.ByteAt(t.q0)
	for !utf8.RuneStart(c) {
		t.q0--
		c, _ = t.buf.ByteAt(t.q0)
	}
	c, _ = t.buf.ByteAt(t.q1)
	for !utf8.RuneStart(c) {
		t.q1--
		c, _ = t.buf.ByteAt(t.q1)
	}

	return t.q0, t.q1, nil
}

// ExpandDot expands the current selection in positive or negative offset. A positive offset expands forwards and a negative expands backwards. Q is 0 or 1, either the left or the right end of the dot.
func (t *Buffer) ExpandDot(q, offset int) {
	if q < 0 || q > 1 {
		return
	}

	if q == 0 {
		t.SetDot(t.q0+offset, t.q1)
	} else {
		t.SetDot(t.q0, t.q1+offset)
	}
}

// Select expands current dot into something useful.
//
// If given pos is adjacent to a quote, parenthese, curly brace or bracket, it tries to select into the matching pair.
//
// If on newline, select the whole line.
//
// Otherwise, select word (longest alphanumeric sequence).
func (t *Buffer) Select(offset int) {
	offset, _ = t.Seek(offset, io.SeekStart)
	start, end := offset, offset

	// space
	//start -= t.PrevSpace(start)
	//end += t.NextSpace(end)

	// word
	start -= t.PrevWord(start)
	end += t.NextWord(end)

	// return a single char selection if no word was found
	if start == end {
		t.Seek(offset, io.SeekStart)
		_, size, _ := t.ReadRune()
		end += size
	}

	// Set dot
	t.SetDot(start, end)
}

func (t *Buffer) NextSpace(offset int) (n int) {
	offset, _ = t.Seek(offset, io.SeekStart)

	r, size, err := t.ReadRune()
	if err != nil {
		return 0
	}
	for !unicode.IsSpace(r) {
		n += size
		r, size, err = t.ReadRune()
		if err != nil {
			if err == io.EOF {
				return n
			}
			return 0
		}
	}

	return n
}

func (t *Buffer) PrevSpace(offset int) (n int) {
	offset, _ = t.Seek(offset, io.SeekStart)

	r, size, err := t.ReadRuneAt(offset)
	if err != nil {
		return 0
	}
	for !unicode.IsSpace(r) {
		r, size, err = t.UnreadRune()
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

func (t *Buffer) NextWord(offset int) (n int) {
	offset, _ = t.Seek(offset, io.SeekStart)

	r, size, err := t.ReadRune()
	if err != nil {
		return 0
	}
	for unicode.IsLetter(r) || unicode.IsDigit(r) {
		n += size
		r, size, err = t.ReadRune()
		if err != nil {
			if err == io.EOF {
				return n
			}
			return 0
		}
	}

	return n
}

func (t *Buffer) PrevWord(offset int) (n int) {
	offset, _ = t.Seek(offset, io.SeekStart)

	r, size, _ := t.ReadRuneAt(offset)
	for unicode.IsLetter(r) || unicode.IsDigit(r) {
		r, size, _ = t.UnreadRune()
		n += size
	}

	if n > 0 {
		n -= size // remove last iteration
	}

	return n
}

// NextDelim returns number of bytes from given offset up until next delimiter.
func (t *Buffer) NextDelim(delim rune, offset int) (n int) {
	t.Seek(offset, io.SeekStart)

	r, size, err := t.ReadRune()
	if err != nil {
		return 0
	}

	for r != delim {
		n += size
		r, size, err = t.ReadRune()
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
func (t *Buffer) PrevDelim(delim rune, offset int) (n int) {
	t.Seek(offset, io.SeekStart)
	r, size, err := t.UnreadRune()
	if err != nil {
		return 0
	}
	n += size

	for r != delim {
		r, size, err = t.UnreadRune()
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

func (t *Buffer) Undo() error {
	c, err := t.history.Undo()
	if err != nil {
		return errors.Wrap(err, "undo")
	}
	t.commit(c)

	// highlight text
	t.SetDot(c.offset, c.offset+len(c.content))

	return nil
}

func (t *Buffer) Redo() error {
	c, err := t.history.Redo()
	if err != nil {
		return errors.Wrap(err, "redo")
	}
	t.commit(c)

	if c.action == HDelete {
		t.SetDot(c.offset, c.offset)
	} else {
		t.SetDot(c.offset+len(c.content), c.offset+len(c.content))
	}
	return nil
}

func (t *Buffer) commit(c Change) (int, error) {
	t.initBuffer()

	switch c.action {
	case HInsert:
		t.buf.Seek(c.offset) // sync gap buffer
		n, err := t.buf.Write([]byte(c.content))
		if err != nil {
			return 0, err
		}
		return n, err
	case HDelete:
		n := len(c.content)
		t.buf.Seek(c.offset + n) // sync gap buffer
		for i := n; i > 0; i-- {
			t.buf.Delete() // gap buffer deletes one byte at a time
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
