package uitcell

import (
	"fmt"

	"github.com/gdamore/tcell"
	"github.com/prodhe/poe/editor"
)

// Window is a tagline and a body with an optional underlying file on disk. It is the main component and handles all events apart from the system wide shortcuts.
type Window struct {
	x, y, w, h int
	bufid      int64
	body       *View
	tagline    *View
	col        *Column // reference to the column where in
	collapsed  bool    // not implemented
	qcnt       int     // quit count
}

// NewWindow returns a fresh window associated with the given filename. For special case filenames, look at the package constants.
func NewWindow(id int64) *Window {
	win := &Window{
		bufid: id,
		body: &View{
			text:         ed.Buffer(id),
			what:         ViewBody,
			style:        bodyStyle,
			cursorStyle:  bodyCursorStyle,
			hilightStyle: bodyHilightStyle,
			tabstop:      4,
			focused:      true,
		},
		tagline: &View{
			text:         &editor.Buffer{},
			what:         ViewTagline,
			style:        tagStyle,
			cursorStyle:  tagCursorStyle,
			hilightStyle: tagHilightStyle,
			tabstop:      4,
		},
	}

	fmt.Fprintf(win.tagline, "%s Del ",
		win.TagName(),
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
		if win.Name() == name {
			return win
		}
	}
	return nil
}

// Resize will set new values for position and width height. Meant to be used on a resize event for proper recalculation during the Draw().
func (win *Window) Resize(x, y, w, h int) {
	win.x, win.y, win.w, win.h = x, y, w, h

	win.tagline.Resize(win.x+3, win.y, win.w-3, 1) // 3 for tagbox
	win.body.Resize(win.x, win.y+1, win.w, win.h-1)
}

func (win *Window) Focus() {
	win.body.focused = true
	//win.tagline.focused = true
}

func (win *Window) UnFocus() {
	win.body.focused = false
	win.tagline.focused = false
}

func (win *Window) Name() string {
	return win.body.text.Name()
}

func (win *Window) TagName() string {
	return win.body.text.Name()
}

func (win *Window) Dir() string {
	d := win.body.text.WorkDir()
	if d == "." {
		return ed.WorkDir() // base path for program
	}
	return d
}

func (win *Window) Flags() [2]rune {
	flags := [2]rune{' ', '-'}
	if win.body.Dirty() {
		flags[0] = '\''
	}
	if win.collapsed {
		flags[1] = '+'
	}
	return flags
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
			_, err := win.body.text.SaveFile()
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
	flags := win.Flags()
	screen.SetContent(win.x, win.y, flags[0], nil, win.tagline.style)
	screen.SetContent(win.x+1, win.y, flags[1], nil, win.tagline.style)
	screen.SetContent(win.x+2, win.y, ' ', nil, win.tagline.style)

	// Tagline
	win.tagline.Draw()

	// Main text buffer
	win.body.Draw()
}

func (win *Window) CanClose() bool {
	if win.body.what == ViewScratch {
		return true
	}
	ok := (!win.body.Dirty() || win.qcnt > 0)
	if !ok {
		name := win.Name()
		if name == FnEmptyWin {
			name = "unnamed buffer"
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
	ed.CloseBuffer(win.bufid)
	win.col.CloseWindow(win)
}
