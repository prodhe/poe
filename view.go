package main

import (
	"io"
	"path/filepath"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell"
	runewidth "github.com/mattn/go-runewidth"
)

const (
	ViewMenu int = iota
	ViewTagline
	ViewBody
	ViewScratch
)

const ClickThreshold = 500 // in milliseconds to count as double click

type View struct {
	x, y, w, h   int
	style        tcell.Style
	cursorStyle  tcell.Style
	hilightStyle tcell.Style
	text         *Text
	scrollpos    int // bytes to skip when drawing content
	opos         int // overflow offset
	tabstop      int
	focused      bool
	what         int
	dirty        bool      // modified since last read/write
	mclicktime   time.Time // last mouse click in time
	mclickpos    int       // byte offset accounting for runes
	mpressed     bool
}

func (v *View) Write(p []byte) (int, error) {
	n, err := v.text.Write(p)
	if err != nil {
		return 0, err
	}
	if v.what == ViewBody {
		v.dirty = true
	}
	v.SetCursor(0, 1) // force scroll if needed
	return n, err
}

func (v *View) Delete() (int, error) {
	// Do not allow deletion beyond what we can see.
	// This forces the user to scroll to visible content.
	if v.text.q0 == v.scrollpos && len(v.text.ReadDot()) == 0 {
		return 0, nil //silent return
	}
	n, err := v.text.Delete()
	if err != nil {
		return n, err
	}
	if v.what == ViewBody {
		v.dirty = true
	}
	v.SetCursor(0, 1) // force scroll
	return n, nil
}

func (v *View) Resize(x, y, w, h int) {
	v.x, v.y, v.w, v.h = x, y, w, h
}

// Byte returns the current byte at start of cursor.
func (v *View) Rune() rune {
	r, _, _ := v.text.ReadRuneAt(v.Cursor())
	return r
}

func (v *View) Cursor() int {
	q0, _ := v.text.Dot()
	return q0
}

func (v *View) SetCursor(pos, whence int) {
	v.text.SeekDot(pos, whence)

	// scroll to cursor if out of screen
	if v.Cursor() < v.scrollpos || v.Cursor() > v.opos {
		if v.Cursor() != v.text.Len() { // do not autoscroll on +1 last byte
			v.ScrollTo(v.Cursor())
		}
	}
}

// XYToOffset translates mouse coordinates in a 2D terminal to the correct byte offset in buffer, accounting for rune length and width.
func (b *View) XYToOffset(x, y int) int {
	offset := b.scrollpos

	// vertical (number of lines)
	for y-b.y > 0 {
		r, _, _ := b.text.ReadRuneAt(offset)
		if r == '\n' {
			offset++
			y--
			continue
		}
		// loop until next line
		xw := 0
		for r != '\n' && xw < b.x+b.w {
			var n int
			r, n, _ = b.text.ReadRuneAt(offset)
			offset += n
			rw := RuneWidth(r)
			if rw == 0 {
				rw = 1
			}
			xw += rw
		}
		y--
	}

	// horizontal
	for x-b.x > 0 {
		r, n, _ := b.text.ReadRuneAt(offset)
		if r == '\n' {
			break
		}
		if r == '\t' {
			x -= b.tabstop - 1 // TODO: modulo count
		}
		offset += n
		rw := RuneWidth(r)
		if rw == 0 {
			rw = 1
		}
		x -= rw
	}

	return offset
}

// Scroll will move the visible part of the buffer in number of lines. Negative means upwards.
func (v *View) Scroll(n int) {
	offset := 0
	if n > 0 {
		for n > 0 {
			if r, _, _ := v.text.ReadRuneAt(v.scrollpos + offset); r == '\n' {
				offset++
			} else {
				offset += v.text.NextDelim('\n', v.scrollpos+offset) + 1
			}
			n--
		}
		v.scrollpos += offset
	}

	if n < 0 {
		offset += v.text.PrevDelim('\n', v.scrollpos)
		for n < 0 {
			offset += v.text.PrevDelim('\n', v.scrollpos-offset)
			n++
		}
		if v.scrollpos-offset > 0 {
			v.scrollpos -= offset - 1
		} else {
			v.scrollpos = 0
		}
	}

	// boundaries
	if v.scrollpos < 0 {
		v.scrollpos = 0
	}
	if v.scrollpos > v.text.Len() {
		v.scrollpos = v.text.Len()
	}
}

// ScrollTo will scroll to an absolute byte offset in the buffer and backwards to the nearest previous newline.
func (v *View) ScrollTo(offset int) {
	offset -= v.text.PrevDelim('\n', offset)
	if offset > 0 {
		offset += 1
	}
	v.scrollpos = offset
	v.Scroll(-(v.h / 3)) // scroll a third page more for context
}

func (b *View) Draw() {
	x, y := b.x, b.y

	if b.text.Len() > 0 {
		b.opos = b.scrollpos // keep track of last visible char/overflow
		b.text.Seek(b.scrollpos, io.SeekStart)
		for i := b.scrollpos; i < b.text.Len(); { // i gets incremented after reading of the rune, to know how many bytes we need to skip
			// line wrap
			if x > b.x+b.w {
				y += 1
				x = b.x
			}

			// stop at visual bottom of view
			if y >= b.y+b.h {
				break
			}

			// default style
			style := b.style

			// highlight cursor
			if (b.text.q0 == b.text.q1 && i == b.text.q0) && b.focused {
				style = b.cursorStyle
			}

			// highlight selection
			if (i >= b.text.q0 && i < b.text.q1) && b.focused {
				style = b.hilightStyle
			}

			// draw rune from buffer
			r, n, err := b.text.ReadRune()
			if err != nil {
				screen.SetContent(x, y, '?', nil, style)
				printMsg("rune [%d]: %s\n", i, err)
				break
			}
			b.opos += n // increment last visible char/overflow
			i += n      // jump past bytes for next run

			// color the entire line if we are in selection
			fillstyle := b.style
			if style == b.hilightStyle {
				fillstyle = b.hilightStyle
			}

			switch r {
			case '\n': // linebreak
				screen.SetContent(x, y, '\n', nil, style)
				for j := x + 1; j <= b.x+b.w; j++ {
					screen.SetContent(j, y, ' ', nil, fillstyle) // fill rest of line
				}
				y += 1
				x = b.x
			case '\t': // show tab until next even tabstop width
				screen.SetContent(x, y, '\t', nil, style)
				x++
				for (x-b.x)%b.tabstop != 0 {
					screen.SetContent(x, y, ' ', nil, fillstyle)
					x++
				}
			default: // print rune
				screen.SetContent(x, y, r, nil, style)
				rw := RuneWidth(r)
				if rw == 2 { // wide runes
					screen.SetContent(x+1, y, ' ', nil, fillstyle)
				}
				if rw == 0 { // control characters
					rw = 1
					screen.SetContent(x, y, RuneWidthZero, nil, unprintableStyle)
				}
				x += rw
			}
		}
	}

	if b.opos != b.text.Len() {
		b.opos--
	}

	// fill out last line if we did not end on a newline
	//c, _ := b.text.buf.ByteAt(b.opos)
	c := b.text.LastRune()
	if c != '\n' && y < b.y+b.h {
		for w := b.x + b.w; w >= x; w-- {
			screen.SetContent(w, y, ' ', nil, b.style)
		}
	}

	// show cursor on EOF
	if b.text.q0 == b.text.Len() && b.focused {
		if x > b.x+b.w {
			x = b.x
			y++
		}
		if y < b.y+b.h {
			screen.SetContent(x, y, ' ', nil, b.cursorStyle)
			x++
		}
	}

	// clear the rest and optionally show a special char as empty line
	for w := b.x + b.w; w >= x; w-- {
		screen.SetContent(w, y, ' ', nil, b.style)
	}
	y++
	if y < b.y+b.h {
		for ; y < b.y+b.h; y++ {
			screen.SetContent(b.x, y, ' ', nil, b.style) // special char
			x = b.x + b.w
			for x >= b.x+1 {
				screen.SetContent(x, y, ' ', nil, bodyStyle)
				x--
			}
		}
	}
}

func (v *View) HandleEvent(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventMouse:
		mx, my := ev.Position()
		pos := v.XYToOffset(mx, my)

		switch btn := ev.Buttons(); btn {
		case tcell.ButtonNone: // on button release
			if v.mpressed {
				v.mpressed = false
			}
		case tcell.Button1:
			if v.mpressed {
				if pos > v.mclickpos {
					v.text.SetDot(v.mclickpos, pos)
				} else {
					// switch q0 and q1
					v.text.SetDot(pos, v.mclickpos)
				}
				return
			}

			v.mpressed = true
			v.mclickpos = pos

			if ev.Modifiers()&tcell.ModAlt != 0 {
				RunCommand(v.text.ReadDot())
				return
			}
			elapsed := ev.When().Sub(v.mclicktime) / time.Millisecond

			if elapsed < ClickThreshold {
				// double click
				v.text.Select(pos)
			} else {
				// single click
				v.SetCursor(pos, 0)
			}
			v.mclicktime = ev.When()
		case tcell.WheelUp: // scrollup
			v.Scroll(-1)
		case tcell.WheelDown: // scrolldown
			v.Scroll(1)
		default:
			printMsg("%#v", btn)
		}
	case *tcell.EventKey:
		key := ev.Key()
		switch key {
		case tcell.KeyCR: // use unix style 0x0A (\n) for new lines
			key = tcell.KeyLF
		case tcell.KeyRight:
			//if ev.Modifiers()&tcell.ModShift != 0 {
			//	v.text.ExpandDot(1, 1)
			//} else {
			v.SetCursor(utf8.RuneLen(v.Rune()), io.SeekCurrent)
			//}
			return
		case tcell.KeyLeft:
			//if ev.Modifiers()&tcell.ModShift != 0 {
			//	v.text.ExpandDot(1, -1)
			//} else {
			v.SetCursor(-1, io.SeekCurrent)
			//}
			return
		case tcell.KeyDown:
			fallthrough
		case tcell.KeyPgDn:
			v.Scroll(v.h / 3)
			return
		case tcell.KeyUp:
			fallthrough
		case tcell.KeyPgUp:
			v.Scroll(-(v.h / 3))
			return
		case tcell.KeyCtrlA: // line start
			offset := v.text.PrevDelim('\n', v.Cursor())
			if offset > 1 && v.Cursor()-offset != 0 {
				offset -= 1
			}
			v.SetCursor(-offset, io.SeekCurrent)
			return
		case tcell.KeyCtrlE: // line end
			if v.Rune() == '\n' {
				v.SetCursor(1, io.SeekCurrent)
				return
			}
			offset := v.text.NextDelim('\n', v.Cursor())
			v.SetCursor(offset, io.SeekCurrent)
			return
		case tcell.KeyCtrlU: // delete line backwards
			offset := v.text.PrevDelim('\n', v.Cursor())
			if offset > 1 && v.Cursor()-offset != 0 {
				offset -= 1
			}
			r, _, _ := v.text.ReadRuneAt(v.Cursor() - offset)
			if r == '\n' && offset > 1 {
				offset -= 1
			}
			v.text.SetDot(v.Cursor()-offset, v.Cursor())
			v.Delete()
			return
		case tcell.KeyCtrlW: // delete word backwards
			offset := v.text.q0 - 1
			c, _ := v.text.buf.ByteAt(offset)
			if unicode.IsSpace(rune(c)) {
				if c == '\n' {
					v.Delete()
					return
				}
				for unicode.IsSpace(rune(c)) && c != '\n' {
					v.Delete()
					if v.text.q0 <= 0 {
						break
					}
					offset--
					c, _ = v.text.buf.ByteAt(offset)
				}
			}
			for !unicode.IsSpace(rune(c)) {
				v.Delete()
				if v.text.q0 <= 0 {
					break
				}
				offset--
				c, _ = v.text.buf.ByteAt(offset)
			}
			return
		case tcell.KeyCtrlZ:
			v.text.Undo()
			return
		case tcell.KeyCtrlY:
			v.text.Redo()
			return
		case tcell.KeyBackspace2: // delete
			fallthrough
		case tcell.KeyCtrlH:
			v.Delete()
			return
		case tcell.KeyCtrlG: // file info/statistics
			printMsg("0x%.4x %q %d,%d/%d\noverflow: %d scroll: %d\ndot: %q\nprev nl: %d\n",
				v.Rune(), v.Rune(),
				v.text.q0, v.text.q1, v.text.Len(),
				v.opos, v.scrollpos,
				v.text.ReadDot(),
				v.text.PrevDelim('\n', v.Cursor()))
			return
		case tcell.KeyCtrlO: // open file/dir
			fn := v.text.ReadDot()
			if fn != FnEmptyWin && fn[0] != filepath.Separator {
				fn = CurWin.Dir() + string(filepath.Separator) + fn
				fn = filepath.Clean(fn)
			}
			CmdOpen(fn)
			return
		case tcell.KeyCtrlR: // run command in dot
			RunCommand(v.text.ReadDot())
			return
		case tcell.KeyCtrlC: // copy to clipboard
			if err := clipboard.WriteAll(v.text.ReadDot()); err != nil {
				printMsg("%s\n", err)
			}
			return
		case tcell.KeyCtrlV: // paste from clipboard
			s, err := clipboard.ReadAll()
			if err != nil {
				printMsg("%s\n", err)
				return
			}
			v.text.Write([]byte(s))
			return
		case tcell.KeyCtrlQ:
			// close entire application if we are in the top menu
			if v.what == ViewMenu {
				RunCommand("Exit")
				return
			}
			// otherwise, just close this window (CurWin)
			RunCommand("Del")
			return
		default:
			// insert
		}

		// insert if no early return
		if key == tcell.KeyRune {
			v.Write([]byte(string(ev.Rune())))
		} else {
			v.Write([]byte{byte(key)})
		}
	}
}

func RuneWidth(r rune) int {
	rw := runewidth.RuneWidth(r)
	if r == '⌘' {
		rw = 2
	}
	return rw
}
