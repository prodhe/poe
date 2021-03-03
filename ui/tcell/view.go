package uitcell

import (
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	tcell "github.com/gdamore/tcell/v2"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/prodhe/poe/editor"
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
	text         *editor.Buffer
	scrollpos    int // bytes to skip when drawing content
	opos         int // overflow offset
	tabstop      int
	focused      bool
	what         int
	mclicktime   time.Time // last mouse click in time
	mclickpos    int       // byte offset accounting for runes
	mpressed     bool
}

func (v *View) Write(p []byte) (int, error) {
	n, err := v.text.Write(p)
	if err != nil {
		return 0, err
	}
	v.SetCursor(0, 1) // force scroll if needed
	return n, err
}

func (v *View) Delete() (int, error) {
	// Do not allow deletion beyond what we can see.
	// This forces the user to scroll to visible content.
	q0, _ := v.text.Dot()
	if q0 == v.scrollpos && len(v.text.ReadDot()) == 0 {
		return 0, nil //silent return
	}
	n, err := v.text.Delete()
	if err != nil {
		return n, err
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

// Cursor returns start of dot.
func (v *View) Cursor() int {
	q0, _ := v.text.Dot()
	return q0
}

// Dirty returns true if the buffer has been written to.
func (v *View) Dirty() bool {
	return v.text.Dirty()
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

// XYToOffset translates mouse coordinates in a 2D terminal to the correct byte offset in buffer, accounting for rune length, width and tabstops.
func (v *View) XYToOffset(x, y int) int {
	offset := v.scrollpos

	// vertical (number of visual lines)
	for y-v.y > 0 {
		r, _, err := v.text.ReadRuneAt(offset)
		if err != nil {
			if err == io.EOF {
				return v.text.Len()
			}
			printMsg("%s\n", err)
			return 0
		}
		if r == '\n' {
			offset++
			y--
			continue
		}
		// loop until next line, either new line or soft wrap at end of window width
		xw := v.x
		for r != '\n' && xw <= v.x+v.w {
			var n int
			r, n, _ = v.text.ReadRuneAt(offset)
			offset += n
			rw := RuneWidth(r)
			if r == '\t' {
				rw = v.tabstop - (xw-v.x)%v.tabstop
			} else if rw == 0 {
				rw = 1
			}

			xw += rw
		}
		y--
	}

	// horizontal
	xw := v.x // for tabstop count
	for x-v.x > 0 {
		r, n, err := v.text.ReadRuneAt(offset)
		if err != nil {
			if err == io.EOF {
				return v.text.Len()
			}
			printMsg("%s\n", err)
			return 0
		}
		if r == '\n' {
			break
		}
		offset += n
		rw := RuneWidth(r)
		if r == '\t' {
			rw = v.tabstop - (xw-v.x)%v.tabstop
		} else if rw == 0 {
			rw = 1
		}
		xw += rw // keep track of tabstop modulo
		x -= rw
	}

	return offset
}

// Scroll will move the visible part of the buffer in number of lines, accounting for soft wraps and tabstops. Negative means upwards.
func (v *View) Scroll(n int) {
	offset := 0

	xw := v.x // for tabstop count and soft wrap
	switch {
	case n > 0: // downwards, next line
		for n > 0 {
			r, size, err := v.text.ReadRuneAt(v.scrollpos + offset)
			if err != nil {
				v.scrollpos = v.text.Len()
				if err == io.EOF {
					break // hit EOF, stop scrolling
				}
				return
			}
			offset += size

			rw := RuneWidth(r)
			if r == '\t' {
				rw = v.tabstop - (xw-v.x)%v.tabstop
			} else if rw == 0 {
				rw = 1
			}
			xw += rw

			if r == '\n' || xw > v.x+v.w { // new line or soft wrap
				n--      // move down
				xw = v.x // reset soft wrap
			}
		}
		v.scrollpos += offset
	case n < 0: // upwards, previous line
		// This is kind of ugly, but it relies on the soft wrap
		// counting in positive scrolling. It will scroll back to the
		// nearest new line character and then scroll forward again
		// until the very last iteration, which is the offset for the previous
		// softwrap/nl.
		for n < 0 {
			start := v.scrollpos                               // save current offset
			v.scrollpos -= v.text.PrevDelim('\n', v.scrollpos) // scroll back
			if start-v.scrollpos == 1 {                        // if it was just a new line, back up one more
				v.scrollpos -= v.text.PrevDelim('\n', v.scrollpos)
			}
			prevlineoffset := v.scrollpos // previous (or one more) new line, may be way back

			for v.scrollpos < start { // scroll one line forward until we're back at current
				prevlineoffset = v.scrollpos // save offset just before we jump forward again
				v.Scroll(1)                  // used for the side effect of setting v.scrollpos
			}
			v.scrollpos = prevlineoffset
			n++
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
	//screen.HideCursor()

	x, y := b.x, b.y

	if b.text.Len() > 0 {
		b.opos = b.scrollpos // keep track of last visible char/overflow
		b.text.Seek(b.scrollpos, io.SeekStart)
		q0, q1 := b.text.Dot()

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

			// highlight cursor if on screen
			if (q0 == q1 && i == q0) && b.focused {
				//style = b.cursorStyle
				screen.ShowCursor(x, y)
			}

			// highlight selection, even if not focused
			if i >= q0 && i < q1 {
				style = b.hilightStyle
				if b.focused {
					screen.HideCursor()
				}
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
	q0, _ := b.text.Dot()
	if q0 == b.text.Len() && b.focused {
		if x > b.x+b.w {
			x = b.x
			y++
		}
		if y < b.y+b.h {
			screen.SetContent(x, y, ' ', nil, b.cursorStyle)
			screen.ShowCursor(x, y)
			x++
		}
	}

	// if we are in focus, we are allowed to hide the central cursor if dot is currently off screen
	if b.focused && (q0 < b.scrollpos || q0 > b.opos) {
		screen.HideCursor()
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

		switch btn := ev.Buttons(); btn {
		case tcell.ButtonNone: // on button release
			if v.mpressed {
				v.mpressed = false
			}
		case tcell.ButtonPrimary:
			pos := v.XYToOffset(mx, my)
			if v.mpressed { // select text via click-n-drag
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

			if ev.Modifiers()&tcell.ModAlt != 0 { // identic code to Btn2
				pos := v.XYToOffset(mx, my)
				// if we clicked inside a current selection, run that one
				q0, q1 := v.text.Dot()
				if pos >= q0 && pos <= q1 && q0 != q1 {
					output := Cmd(v.text.ReadDot())
					if output != "" {
						printMsg(output)
					}
					return
				}

				// otherwise, select non-space chars under mouse and run that
				p := pos - v.text.PrevSpace(pos)
				n := pos + v.text.NextSpace(pos)
				v.text.SetDot(p, n)
				str := strings.Trim(v.text.ReadDot(), "\n\t ")
				v.text.SetDot(q0, q1)
				output := Cmd(str)
				if output != "" {
					printMsg(output)
				}
				return
			}

			if ev.Modifiers()&tcell.ModShift != 0 { // identic code to Btn3
				pos := v.XYToOffset(mx, my)
				// if we clicked inside a current selection, open that one
				q0, q1 := v.text.Dot()
				if pos >= q0 && pos <= q1 && q0 != q1 {
					CmdOpen(v.text.ReadDot())
					return
				}

				// otherwise, select everything inside surround spaces and open that
				p := pos - v.text.PrevSpace(pos)
				n := pos + v.text.NextSpace(pos)
				v.text.SetDot(p, n)
				fn := strings.Trim(v.text.ReadDot(), "\n\t ")
				v.text.SetDot(q0, q1)
				if fn == "" { // if it is still blank, abort
					return
				}
				if fn != "" && fn[0] != filepath.Separator {
					fn = CurWin.Dir() + string(filepath.Separator) + fn
					fn = filepath.Clean(fn)
				}
				CmdOpen(fn)
				return
			}

			elapsed := ev.When().Sub(v.mclicktime) / time.Millisecond

			if elapsed < ClickThreshold {
				// double click
				v.text.Select(pos)
			} else {
				// single click
				v.SetCursor(pos, 0)
				//screen.ShowCursor(3, 3)
			}
			v.mclicktime = ev.When()
		case tcell.WheelUp: // scrollup
			v.Scroll(-1)
		case tcell.WheelDown: // scrolldown
			v.Scroll(1)
		case tcell.ButtonMiddle: // middle click
			pos := v.XYToOffset(mx, my)
			// if we clicked inside a current selection, run that one
			q0, q1 := v.text.Dot()
			if pos >= q0 && pos <= q1 && q0 != q1 {
				Cmd(v.text.ReadDot())
				return
			}

			// otherwise, select non-space chars under mouse and run that
			p := pos - v.text.PrevSpace(pos)
			n := pos + v.text.NextSpace(pos)
			v.text.SetDot(p, n)
			fn := strings.Trim(v.text.ReadDot(), "\n\t ")
			v.text.SetDot(q0, q1)
			Cmd(fn)
			return
		case tcell.ButtonSecondary: // right click
			pos := v.XYToOffset(mx, my)
			// if we clicked inside a current selection, open that one
			q0, q1 := v.text.Dot()
			if pos >= q0 && pos <= q1 && q0 != q1 {
				CmdOpen(v.text.ReadDot())
				return
			}

			// otherwise, select everything inside surround spaces and open that
			p := pos - v.text.PrevSpace(pos)
			n := pos + v.text.NextSpace(pos)
			v.text.SetDot(p, n)
			fn := strings.Trim(v.text.ReadDot(), "\n\t ")
			v.text.SetDot(q0, q1)
			if fn == "" { // if it is still blank, abort
				return
			}
			if fn != "" && fn[0] != filepath.Separator {
				fn = CurWin.Dir() + string(filepath.Separator) + fn
				fn = filepath.Clean(fn)
			}
			CmdOpen(fn)
			return
		default:
			printMsg("%#v", btn)
		}
	case *tcell.EventKey:
		key := ev.Key()
		switch key {
		case tcell.KeyCR: // use unix style 0x0A (\n) for new lines
			key = tcell.KeyLF
		case tcell.KeyRight:
			_, q1 := v.text.Dot()
			v.SetCursor(q1, io.SeekStart)
			v.SetCursor(utf8.RuneLen(v.Rune()), io.SeekCurrent)
			return
		case tcell.KeyLeft:
			v.SetCursor(-1, io.SeekCurrent)
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
			if v.text.ReadDot() != "" {
				v.Delete() // delete current selection first
			}
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
			if v.text.ReadDot() != "" {
				v.Delete() // delete current selection first
			}
			startpos := v.Cursor()
			offset := v.text.PrevWord(v.Cursor())
			if offset == 0 {
				v.SetCursor(-1, io.SeekCurrent)
				offset = v.text.PrevWord(v.Cursor())
			}
			v.text.SetDot(v.Cursor()-offset, startpos)
			v.Delete()
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
			sw, sh := screen.Size()
			printMsg("0x%.4x %q len %d\nbasedir: %s\nwindir: %s\nname: %s\nw: %d h: %d sw: %d sh: %d\n",
				v.Rune(), v.Rune(),
				v.text.Len(),
				ed.WorkDir(), CurWin.Dir(), CurWin.Name(),
				CurWin.w, CurWin.h, sh, sw)
			return
		case tcell.KeyCtrlO: // open file/dir
			fn := v.text.ReadDot()
			if fn == "" { // select all non-space characters
				curpos := v.Cursor()
				p := curpos - v.text.PrevSpace(curpos)
				n := curpos + v.text.NextSpace(curpos)
				v.text.SetDot(p, n)
				fn = strings.Trim(v.text.ReadDot(), "\n\t ")
				v.SetCursor(curpos, io.SeekStart)
				if fn == "" { // if it is still blank, abort
					return
				}
			}
			if fn != "" && fn[0] != filepath.Separator {
				fn = CurWin.Dir() + string(filepath.Separator) + fn
				fn = filepath.Clean(fn)
			}
			CmdOpen(fn)
			return
		case tcell.KeyCtrlN: // new column
			CmdNewcol()
			return
		case tcell.KeyCtrlR: // run command in dot
			cmd := v.text.ReadDot()
			if cmd == "" { // select all non-space characters
				curpos := v.Cursor()
				p := curpos - v.text.PrevSpace(curpos)
				n := curpos + v.text.NextSpace(curpos)
				v.text.SetDot(p, n)
				cmd = strings.Trim(v.text.ReadDot(), "\n\t ")
				v.SetCursor(curpos, io.SeekStart)
				if cmd == "" { // if it is still blank, abort
					return
				}
			}
			res := Cmd(cmd)
			printMsg("%s\n", res)
			return
		case tcell.KeyCtrlC: // copy to clipboard
			str := v.text.ReadDot()
			if str == "" {
				return
			}
			if err := clipboard.WriteAll(str); err != nil {
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
		case tcell.KeyCtrlQ: // close window
			CmdDel()
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
