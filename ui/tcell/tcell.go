package uitcell

import (
	"fmt"
	"path/filepath"
	"strings"

	tcell "github.com/gdamore/tcell/v2"
	"github.com/prodhe/poe/editor"
)

const (
	FnMessageWin  = "+poe"
	FnEmptyWin    = ""
	RuneWidthZero = '?'
)

var (
	screen    tcell.Screen
	ed        editor.Editor
	workspace *Workspace
	CurCol    *Column
	CurWin    *Window

	poecmds map[string]commandFunc

	quit   chan bool
	events chan tcell.Event
)

type commandFunc func()

type Tcell struct{}

func (t *Tcell) Init(e editor.Editor) error {
	ed = e

	if err := initScreen(); err != nil {
		return err
	}

	if err := initStyles(); err != nil {
		return err
	}

	if err := setStyleAcme(); err != nil {
		return err
	}

	if err := initWorkspace(); err != nil {
		return err
	}

	if ed.Len() == 0 {
		ed.LoadBuffers([]string{"."})
	}

	if err := initWindows(); err != nil {
		return err
	}

	initCommands()

	quit = make(chan bool, 1)
	events = make(chan tcell.Event, 100)

	return nil
}

func (t *Tcell) Close() {
	if screen == nil {
		return
	}
	screen.DisableMouse()
	screen.Fini()
}

func printMsg(format string, a ...interface{}) {
	// get output window
	var poename string

	if CurWin != nil {
		poename = CurWin.Dir()
	} else {
		poename = ed.WorkDir()
	}
	poename += string(filepath.Separator) + FnMessageWin
	poename = filepath.Clean(poename)

	poewin := FindWindow(poename)

	if poewin == nil {
		id, buf := ed.NewBuffer()
		buf.NewFile(poename)
		poewin = NewWindow(id)
		poewin.body.what = ViewScratch

		if len(workspace.cols) < 2 {
			workspace.AddCol()
		}
		workspace.LastCol().AddWindow(poewin)
	}

	poewin.body.SetCursor(poewin.body.text.Len(), 0)

	if a == nil {
		fmt.Fprintf(poewin.body, format)
		return
	}
	fmt.Fprintf(poewin.body, format, a...)
}

func initScreen() error {
	var err error
	screen, err = tcell.NewScreen()
	if err != nil {
		return err
	}
	if err = screen.Init(); err != nil {
		return err
	}
	screen.SetStyle(bodyStyle)
	screen.EnableMouse()
	screen.Sync()
	return nil
}

func initWorkspace() error {
	workspace = &Workspace{ // first resize event will set proper dimensions
		tagline: &View{
			text:         &editor.Buffer{},
			what:         ViewMenu,
			style:        tagStyle,
			cursorStyle:  tagCursorStyle,
			hilightStyle: tagHilightStyle,
			tabstop:      4,
		},
	}
	fmt.Fprintf(workspace.tagline, "%s", "Newcol Exit ")
	workspace.AddCol()
	if ids, _ := ed.Buffers(); len(ids) == 0 {
		workspace.AddCol()
	}
	return nil
}

func initWindows() error {
	ids, _ := ed.Buffers()
	for _, id := range ids {
		win := NewWindow(id)
		workspace.LastCol().AddWindow(win)
		CurWin = win
	}
	return nil
}

func initCommands() {
	poecmds = map[string]commandFunc{
		"Newcol": CmdNewcol,
		"Delcol": CmdDelcol,
		"New":    CmdNew,
		"Del":    CmdDel,
		"Get":    CmdGet,
		"Exit":   CmdExit,
	}
}

func (t *Tcell) redraw() {
	workspace.Draw()
	screen.Show()
}

func (t *Tcell) Listen() {
	go func() {
		for {
			events <- screen.PollEvent()
		}
	}()

outer:
	for {
		// draw
		t.redraw()

		var event tcell.Event

		select {
		case <-quit:
			break outer
		case event = <-events:
		}

		for event != nil {
			switch e := event.(type) {
			case *tcell.EventResize:
				w, h := screen.Size()
				workspace.Resize(0, 0, w, h)
				screen.Clear()
				screen.Sync()
			case *tcell.EventKey: // system wide shortcuts
				switch e.Key() {
				case tcell.KeyCtrlL: // refresh terminal
					screen.Clear()
					screen.Sync()
				default: // let the focused view handle event
					if CurWin != nil {
						CurWin.HandleEvent(e)
					} else if CurCol != nil {
						CurCol.tagline.HandleEvent(e)
					} else {
						workspace.tagline.HandleEvent(e)
					}
				}
			case *tcell.EventMouse:
				screen.HideCursor()
				mx, my := e.Position()
				if CurCol != nil {
					CurCol.tagline.focused = false
				}
				if CurWin != nil {
					CurWin.UnFocus()
				}
				CurCol = nil
				CurWin = nil

				if my < 1 {
					workspace.tagline.focused = true
					workspace.tagline.HandleEvent(e)
				} else {
					workspace.tagline.focused = false
				}

				for _, col := range workspace.cols {
					if mx >= col.x && mx < col.x+col.w &&
						my >= col.y && my < col.y+col.h {
						CurCol = col
					}
				}

				if CurCol != nil {
					if my == CurCol.y {
						CurCol.tagline.focused = true
						CurCol.tagline.HandleEvent(e)
					} else {
						CurCol.tagline.focused = false
					}
				}

				// find which window to send the event to
				for _, win := range AllWindows() {
					win.UnFocus()
					if mx >= win.x && mx < win.x+win.w &&
						my >= win.y && my < win.y+win.h {
						CurWin = win
					}
				}

				if CurWin != nil {
					CurWin.Focus()
					CurWin.HandleEvent(e)
				}
			}

			select {
			case event = <-events:
			default:
				event = nil
			}
		}
	}
}

func Cmd(input string) string {
	if input == "" {
		return ""
	}

	input = strings.Trim(input, "\t\n ")

	// check poe default commands
	cmd := strings.Split(string(input), " ")
	if fn, ok := poecmds[cmd[0]]; ok {
		fn()
		return ""
	}

	// Edit shortcuts for external commands and piping
	switch input[0] {
	case '!', '<', '>', '|':
		return ed.Edit(CurWin.bufid, input)
	}

	return ed.Edit(CurWin.bufid, "!"+input)
}

func CmdOpen(fn string) {
	screen.Clear()
	var win *Window
	win = FindWindow(fn)

	if win != nil { //only load windows that do no already exists
		return
	}

	id, buf := ed.NewBuffer()
	buf.NewFile(fn)
	buf.ReadFile()
	win = NewWindow(id)
	var col *Column
	if !buf.IsDir() {
		// add file window second to last or the first
		if len(workspace.cols) > 2 {
			col = workspace.Col(len(workspace.cols) - 2)
		} else {
			col = workspace.Col(0)
		}
	} else {
		// add all dirs to last column
		col = workspace.LastCol()
	}
	col.AddWindow(win)
}

func CmdNew() {
	screen.Clear()
	id, _ := ed.NewBuffer()
	win := NewWindow(id)
	CurCol.AddWindow(win)
}

func CmdDel() {
	CurWin.Close()
	screen.HideCursor()
}

func CmdNewcol() {
	workspace.AddCol()
	screen.Clear()
}

func CmdDelcol() {
	if len(workspace.cols) < 2 {
		CmdExit()
	} else if CurCol != nil {
		workspace.CloseCol(CurCol)
	} else if CurWin != nil {
		workspace.CloseCol(CurWin.col)
	}
	screen.Clear()
}

func CmdGet() {
	screen.Clear()
	wins := AllWindows()
	for _, win := range wins {
		if win.tagline.focused || win.body.focused {
			q0, q1 := win.body.text.Dot()
			win.body.text.Destroy()
			win.body.text.ReadFile()
			win.body.text.SetDot(q0, q1)
		}
	}
}

func CmdExit() {
	exit := true
	wins := AllWindows()
	for _, win := range wins {
		if !win.CanClose() {
			exit = false
		}
	}
	if exit {
		quit <- true
	}
}
