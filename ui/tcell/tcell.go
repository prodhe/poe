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

	if err := initWorkspace(); err != nil {
		return err
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
	poename := CurWin.Dir() + string(filepath.Separator) + FnMessageWin
	poename = filepath.Clean(poename)

	poewin := FindWindow(poename)

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
		CurWin = win
	}
	return nil
}

func initCommands() {
	poecmds = map[string]commandFunc{
		"New":    CmdNew,
		"Newcol": CmdNewcol,
		"Del":    CmdDel,
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
				screen.Sync()
			case *tcell.EventKey: // system wide shortcuts
				switch e.Key() {
				case tcell.KeyCtrlL: // refresh terminal
					screen.Clear()
					screen.Sync()
				default: // let the focused view handle event
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
	if win == nil { //only load windows that do no already exists
		id, buf := ed.NewBuffer()
		buf.NewFile(fn)
		buf.ReadFile()
		win := NewWindow(id)
		workspace.LastCol().AddWindow(win)
	}
}

func CmdNew() {
	screen.Clear()
	id, _ := ed.NewBuffer()
	win := NewWindow(id)
	workspace.LastCol().AddWindow(win)
}

func CmdDel() {
	if len(AllWindows()) == 1 {
		CmdExit()
		return
	}
	CurWin.Close()
}

func CmdNewcol() {
	workspace.AddCol()
	CmdNew()
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
