package main

type Workspace struct {
	x, y, w, h int
	cols       []*Column
}

type Column struct {
	x, y, w, h int
	windows    []*Window
}

// Add adds a new column and resizes. It returns the index of the newly created column.
func (wrk *Workspace) AddCol() {
	nx, ny := wrk.x, wrk.y
	nw, nh := wrk.w, wrk.h

	if len(wrk.cols) > 0 {
		nx = wrk.cols[len(wrk.cols)-1].x + wrk.cols[len(wrk.cols)-1].w/2
		nw = wrk.cols[len(wrk.cols)-1].w / 2
		wrk.cols[len(wrk.cols)-1].w /= 2
	}

	newcol := &Column{nx, ny, nw, nh, nil}
	wrk.cols = append(wrk.cols, newcol)

	wrk.Resize(wrk.x, wrk.y, wrk.w, wrk.h) // for re-arranging side effects
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
	return wrk.cols[i]
}

func (wrk *Workspace) LastCol() *Column {
	return wrk.cols[len(wrk.cols)-1]
}

func (wrk *Workspace) Resize(x, y, w, h int) {
	wrk.x, wrk.y, wrk.w, wrk.h = x, y, w, h

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
			wrk.cols[i].Resize(x, y, (w/n)+remainder-(n-1)-firstvertline, h)
			continue
		}
		// +i-n-1 on x so we do not draw on last vert line of previous col
		wrk.cols[i].Resize((w/n)*i+remainder+i-(n-1), y, (w/n)-1, h)
	}
}

func (wrk *Workspace) Draw() {
	for _, col := range wrk.cols {
		col.Draw()

		// draw vertical lines between cols
		for x, y := col.x+col.w+1, wrk.y; y < wrk.y+wrk.h; y++ {
			screen.SetContent(x, y, '|', nil, vertlineStyle)
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

	// If we deleted the current window (probably), select another
	if CurWin == w {
		all := AllWindows()

		// If we are out of windows in our own column, pick another or exit
		if len(c.windows) > 0 {
			CurWin = c.windows[j-1]
		} else {
			// remove column
			workspace.CloseCol(c)

			// clear clutter
			screen.Clear()

			// find another window to focus or exit
			if len(all) > 0 {
				CurWin = AllWindows()[0]

			} else {
				RunCommand("Exit")
			}
		}

		// if the only win left is the message win, close all
		if len(all) == 1 && CurWin.Name() == FnMessageWin {
			RunCommand("Exit")
		}
	}

	c.ResizeWindows()
}

func (c *Column) Resize(x, y, w, h int) {
	c.x, c.y = x, y
	c.w, c.h = w, h

	c.ResizeWindows()
}

func (c *Column) ResizeWindows() {
	var n int
	for _, win := range c.windows {
		if !win.hidden {
			n++
		}
	}

	var remainder int
	if n > 0 {
		remainder = c.h % n
	}
	for i, win := range c.windows {
		if i == 0 {
			win.Resize(c.x, c.y, c.w, (c.h/n)+remainder)
			continue
		}
		win.Resize(c.x, c.y+(c.h/n)*i+remainder, c.w, c.h/n)
	}
}

func (c *Column) Draw() {
	for _, win := range c.windows {
		if !win.hidden {
			win.Draw()
		}
	}
}
