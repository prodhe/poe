package uitcell

import (
	"fmt"
	"path/filepath"

	"github.com/gdamore/tcell"
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
	menu      *View
	workspace *Workspace
	CurWin    *Window

	quit   chan bool
	events chan tcell.Event
)

type Tcell struct{}

func (t *Tcell) Init(e editor.Editor) error {
	ed = e
	if err := initScreen(); err != nil {
		return err
	}

	if err := initStyles(); err != nil {
		return err
	}

	if err := initMenu(); err != nil {
		return err
	}

	if err := initWorkspace(); err != nil {
		return err
	}

	if err := initWindows(); err != nil {
		return err
	}

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
	var poewin *Window
	for _, win := range AllWindows() {
		poename := win.Dir() + string(filepath.Separator) + FnMessageWin
		poename = filepath.Clean(poename)
		if win.Name() == poename && CurWin.Dir() == win.Dir() {
			poewin = win
		}
	}

	if poewin == nil {
		id, buf := ed.NewBuffer()
		buf.NewFile(CurWin.Dir() + string(filepath.Separator) + FnMessageWin)
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

func initMenu() error {
	menu = &View{
		text:         &editor.Buffer{},
		what:         ViewMenu,
		style:        bodyStyle,
		cursorStyle:  bodyCursorStyle,
		hilightStyle: bodyHilightStyle,
		tabstop:      4,
	}
	fmt.Fprintf(menu, "Exit New Newcol")
	return nil
}

func initWorkspace() error {
	workspace = &Workspace{} // first resize event will set proper dimensions
	workspace.AddCol()
	return nil
}

func initWindows() error {
	ids, _ := ed.Buffers()
	for _, id := range ids {
		win := NewWindow(id)
		workspace.LastCol().AddWindow(win)
	}
	return nil
}

func (t *Tcell) redraw() {
	menu.Draw()
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
				menu.Resize(0, 0, w, 1)
				workspace.Resize(0, 1, w, h-1)
				//screen.Clear()
				screen.Sync()
			case *tcell.EventKey: // system wide shortcuts
				switch e.Key() {
				case tcell.KeyCtrlL: // refresh terminal
					screen.Clear()
					screen.Sync()
				default: // let the focused view handle event
					if menu.focused {
						menu.HandleEvent(e)
						break
					}
					if CurWin != nil {
						CurWin.HandleEvent(e)
					}
				}
			case *tcell.EventMouse:
				mx, my := e.Position()

				// find which window to send the event to
				for _, win := range AllWindows() {
					win.UnFocus()
					if mx >= win.x && mx < win.x+win.w &&
						my >= win.y && my < win.y+win.h {
						CurWin = win
					}
				}

				// check if we are in the menu
				menu.focused = false
				if my < 1 {
					menu.focused = true
					menu.HandleEvent(e)
					break
				}

				if CurWin != nil {
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

func CmdOpen(fn string) {
	screen.Clear()
	var win *Window
	win = FindWindow(fn)
	if win == nil { //only load windows that do no already exists
		id, buf := ed.NewBuffer()
		buf.NewFile(fn)
		buf.ReadFile()
		win := NewWindow(id)
		workspace.LastCol().AddWindow(win)
	}
}

func CmdNew(args string) {
	screen.Clear()
	id, _ := ed.NewBuffer()
	win := NewWindow(id)
	workspace.LastCol().AddWindow(win)
}

func CmdDel() {
	CurWin.Close()
}

func CmdExit() {
	exit := true
	wins := AllWindows()
	for _, win := range wins {
		if !win.CanClose() {
			exit = false
		}
	}
	if exit || len(wins) == 0 {
		quit <- true
	}
}
