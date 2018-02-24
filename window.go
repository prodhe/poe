package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell"
	"github.com/pkg/errors"
	"github.com/prodhe/poe/gapbuffer"
)

// File holds information about the file on disk. It is managed by Window and the bytes on disk are optionally loaded into a Text.
type File struct {
	name   string
	read   bool      // true if file has been read
	mtime  time.Time // of file when last read/written
	sha256 string    // of file when last read/written
}

// Window is a tagline and a body with an optional underlying file on disk. It is the main component and handles all events apart from the system wide shortcuts.
type Window struct {
	x, y, w, h int
	file       *File
	isdir      bool
	body       *View
	tagline    *View
	col        *Column // reference to the column where in
	hidden     bool
	collapsed  bool // not implemented
	qcnt       int  // quit count
}

// NewWindow returns a fresh window associated with the given filename. For special case filenames, look at the package constants.
func NewWindow(fn string) *Window {
	fnabs := fn
	if fn != FnEmptyWin {
		var err error
		fnabs, err = filepath.Abs(fn)
		if err != nil {
			printMsg("%s\n", err)
			fnabs = FnEmptyWin
		}
	}
	win := &Window{
		body: &View{
			text:         &Text{buf: gapbuffer.New()},
			what:         ViewBody,
			style:        bodyStyle,
			cursorStyle:  bodyCursorStyle,
			hilightStyle: bodyHilightStyle,
			tabstop:      4,
			focused:      true,
		},
		tagline: &View{
			text:         &Text{buf: gapbuffer.New()},
			what:         ViewTagline,
			style:        tagStyle,
			cursorStyle:  tagCursorStyle,
			hilightStyle: tagHilightStyle,
			tabstop:      4,
		},
		file: &File{name: fnabs},
	}

	fmt.Fprintf(win.tagline, "%s%s Del ",
		win.Flags(),
		win.Name(),
	)

	return win
}

func AllWindows() []*Window {
	var ws []*Window
	for _, col := range workspace.cols {
		ws = append(ws, col.windows...)
	}
	return ws
}

// FindWindow searches for the given name in current windows and returns a pointer if one already exists. Nil otherwise. Name assumes to be an absolute path.
func FindWindow(name string) *Window {
	for _, win := range AllWindows() {
		if win.NameAbs() == name {
			return win
		}
	}
	return nil
}

func (win *Window) LoadBuffer() bool {
	if win.file.read || win.Name() == FnEmptyWin || win.body.what == ViewScratch {
		return false
	}

	info, err := os.Stat(win.file.name)
	if err != nil {
		// if the file exists, print why we could not open it
		// otherwise just close silently
		if os.IsExist(err) {
			printMsg("%s\n", err)
		}
		win.Close()
		return false
	}

	// name is a directory; list it's content into the buffer
	if info.IsDir() {
		files, err := ioutil.ReadDir(win.file.name)
		if err != nil {
			printMsg("%s\n", err)
			return false
		}

		win.body.what = ViewScratch
		win.isdir = true

		// list files in dir
		for _, f := range files {
			dirchar := ""
			if f.IsDir() {
				dirchar = string(filepath.Separator)
			}
			fmt.Fprintf(win.body.text.buf, "%s%s\n", f.Name(), dirchar)
		}
		return true
	}

	// name is a file
	fh, err := os.OpenFile(win.file.name, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		printMsg("%s\n", err)
		return false
	}
	defer fh.Close()

	if _, err := io.Copy(win.body.text.buf, fh); err != nil {
		printMsg("%s\n", err)
		return false
	}
	fh.Seek(0, 0)

	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		printMsg("%s\n", err)
		return false
	}
	win.file.sha256 = fmt.Sprintf("%x", h.Sum(nil))

	win.file.mtime = info.ModTime()
	win.file.read = true

	win.body.SetCursor(0, 0)

	return true
}

// SaveFile replaces disk file with buffer content. Returns error if no disk file is set.
func (win *Window) SaveFile() (int, error) {
	if win.file.name == FnEmptyWin {
		return 0, errors.New("no filename")
	}

	if win.Name() == FnMessageWin {
		return 0, nil
	}

	if win.isdir {
		return 0, nil
	}

	f, err := os.OpenFile(win.NameAbs(), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, errors.Wrap(err, "sha256")
	}
	hhex := fmt.Sprintf("%x", h.Sum(nil))

	if hhex != win.file.sha256 {
		return 0, errors.Errorf("file has been modified outside of poe")
	}

	n, err := f.WriteAt(win.body.text.buf.Bytes(), 0)
	if err != nil {
		return 0, err
	}
	f.Truncate(int64(n))
	f.Sync()

	win.file.sha256 = fmt.Sprintf("%x", sha256.Sum256(win.body.text.buf.Bytes()))

	info, err := f.Stat()
	if err != nil {
		return n, err
	}
	win.file.mtime = info.ModTime()

	win.body.dirty = false

	return n, nil
}

// Name returns either the file from disk name or empty string if the buffer has no disk counterpart.
func (win *Window) Name() string {
	if win.file.name == FnEmptyWin {
		return FnEmptyWin
	}
	s := win.NameAbs()
	if strings.HasPrefix(s, baseDir) {
		s = "." + strings.TrimPrefix(s, baseDir)
		s = filepath.Clean(s)
	}
	return s
}

func (win *Window) NameAbs() string {
	if win.file.name == FnEmptyWin {
		return ""
	}
	s, _ := filepath.Abs(win.file.name)
	return s
}

func (win *Window) NameFromTag() string {
	tstr := win.tagline.text.String()
	if tstr == "" {
		return ""
	}
	return strings.Split(tstr, " ")[0]
}

// Dir returns the working directory of current window, without trailing path separator.
func (win *Window) Dir() string {
	if win.isdir {
		return win.NameAbs()
	}
	if win.Name() == FnEmptyWin {
		return baseDir
	}
	return filepath.Dir(win.NameAbs())
}

// Flags returns the tagline flags for the window.
//
// modified: '
func (win *Window) Flags() string {
	var flags string
	if win.body.dirty {
		flags += "'"
	}
	return flags
}

// Resize will set new values for position and width height. Meant to be used on a resize event for proper recalculation during the Draw().
func (win *Window) Resize(x, y, w, h int) {
	win.x, win.y, win.w, win.h = x, y, w, h

	win.tagline.Resize(win.x, win.y, win.w, 1)
	win.body.Resize(win.x, win.y+1, win.w, win.h-1)
}

func (win *Window) UnFocus() {
	win.body.focused = true
	win.tagline.focused = false
}

func (win *Window) HandleEvent(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventMouse:
		_, my := ev.Position()

		// Set focus to either tagline or body
		if my > win.tagline.y+win.tagline.h-1 {
			win.tagline.focused = false
		} else {
			win.tagline.focused = true
		}
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyCtrlS: // save
			_, err := win.SaveFile()
			if err != nil {
				printMsg("%s\n", err)
			}
			return
		}
	}

	// Pass along the event down to current view, if we have not already done something and returned in the switch above.
	if win.tagline.focused {
		win.tagline.HandleEvent(ev)
	} else {
		win.body.HandleEvent(ev)
	}
}

func (win *Window) Draw() {
	// Tagline
	win.tagline.Draw()

	// Main text buffer
	win.body.Draw()
}

func (win *Window) CanClose() bool {
	ok := !win.body.dirty || win.qcnt > 0
	if !ok {
		name := win.Name()
		if name == FnEmptyWin {
			name = "unnamed file"
		}
		printMsg("%s modified\n", name)
		win.qcnt++
	}
	return ok
}

func (win *Window) Close() {
	if !win.CanClose() {
		return
	}
	win.col.CloseWindow(win)
}
