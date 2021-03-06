package uitcell

import (
	"fmt"

	"github.com/prodhe/poe/editor"
)

const (
	RuneVerticalLine = '\u2502' // \u007c = |, \u23b8, \u2502
)

type Workspace struct {
	x, y, w, h int
	tagline    *View
	cols       []*Column
}

type Column struct {
	x, y, w, h int
	tagline    *View
	windows    []*Window
}

// Add adds a new column and resizes.
func (wrk *Workspace) AddCol() *Column {
	nx, ny := wrk.x, wrk.y
	nw, nh := wrk.w, wrk.h

	if len(wrk.cols) > 0 {
		nx = wrk.cols[len(wrk.cols)-1].x + wrk.cols[len(wrk.cols)-1].w/2
		nw = wrk.cols[len(wrk.cols)-1].w / 2
		wrk.cols[len(wrk.cols)-1].w /= 2
	}

	newcol := &Column{
		x: nx,
		y: ny,
		w: nw,
		h: nh,
		tagline: &View{
			text:         &editor.Buffer{},
			what:         ViewColumn,
			style:        bodyStyle,
			cursorStyle:  bodyCursorStyle,
			hilightStyle: bodyHilightStyle,
			tabstop:      4,
		},
		windows: nil,
	}
	fmt.Fprintf(newcol.tagline, "%s", "New Delcol ")

	if len(wrk.cols) > 1 {
		wrk.cols = append(wrk.cols[:len(wrk.cols)-1], newcol, wrk.cols[len(wrk.cols)-1])
	} else {
		wrk.cols = append(wrk.cols, newcol)
	}

	wrk.Resize(wrk.x, wrk.y, wrk.w, wrk.h) // for re-arranging side effects

	return newcol
}

func (wrk *Workspace) CloseCol(c *Column) {
	var j int
	for _, col := range wrk.cols {
		if col != c {
			wrk.cols[j] = col
			j++
		}
	}
	wrk.cols = wrk.cols[:j]

	wrk.Resize(wrk.x, wrk.y, wrk.w, wrk.h) // for re-arranging side effects
}

func (wrk *Workspace) Col(i int) *Column {
	if i > len(wrk.cols)-1 {
		return nil
	}
	return wrk.cols[i]
}

func (wrk *Workspace) LastCol() *Column {
	return wrk.cols[len(wrk.cols)-1]
}

func (wrk *Workspace) Resize(x, y, w, h int) {
	wrk.x, wrk.y, wrk.w, wrk.h = x, y, w, h
	wrk.tagline.x, wrk.tagline.y, wrk.tagline.w = x, y, w
	wrk.tagline.h = 1

	n := len(wrk.cols)
	if n == 0 {
		return
	}

	var remainder int
	if n > 0 {
		remainder = w % (n)
	}
	for i := range wrk.cols {
		if i == 0 {
			var firstvertline int
			if n > 1 {
				firstvertline = 1
			}
			wrk.cols[i].Resize(x, y+1, (w/n)+remainder-(n-1)-firstvertline, h)
			continue
		}
		// +i-n-1 on x so we do not draw on last vert line of previous col
		wrk.cols[i].Resize((w/n)*i+remainder+i-(n-1), y+1, (w/n)-1, h)
	}
}

func (wrk *Workspace) Draw() {
	wrk.tagline.Draw()
	for _, col := range wrk.cols {
		col.Draw()

		// draw vertical lines between cols
		for x, y := col.x+col.w+1, wrk.y+1; y < wrk.y+wrk.h; y++ {
			screen.SetContent(x, y, RuneVerticalLine, nil, vertlineStyle)
		}
	}
}

func (c *Column) AddWindow(win *Window) {
	win.col = c
	c.windows = append(c.windows, win)
	c.ResizeWindows()
}

func (c *Column) CloseWindow(w *Window) {
	var j int
	for _, win := range c.windows {
		if win != w {
			c.windows[j] = win
			j++
		}
	}
	c.windows = c.windows[:j]

	if CurWin == w {
		// If we are not out of windows in our own column, pick another or do nothing
		if len(c.windows) > 0 {
			CurWin = c.windows[j-1]
		} else {
			// clear clutter
			screen.Clear()

			return
		}
	}

	c.ResizeWindows()
}

func (c *Column) Resize(x, y, w, h int) {
	c.x, c.y = x, y
	c.w, c.h = w, h
	c.tagline.x, c.tagline.y = x, y
	c.tagline.w, c.tagline.h = w, 1

	c.ResizeWindows()
}

func (c *Column) ResizeWindows() {
	n := len(c.windows)

	var remainder int
	if n > 0 {
		remainder = c.h % n
	}
	for i, win := range c.windows {
		if i == 0 {
			win.Resize(c.x, c.y+1, c.w, (c.h/n)+remainder)
			continue
		}
		win.Resize(c.x, c.y+1+(c.h/n)*i+remainder, c.w, c.h/n)
	}
}

func (c *Column) Draw() {
	c.tagline.Draw()
	for _, win := range c.windows {
		win.Draw()
	}
	if len(c.windows) == 0 {
		x, y := c.x, c.y+1
		for ; y < c.y+c.h; y++ {
			x = c.x + c.w
			for x >= c.x {
				screen.SetContent(x, y, ' ', nil, bodyStyle)
				x--
			}
		}
	}
}
