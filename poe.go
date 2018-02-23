package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gdamore/tcell"
	"github.com/prodhe/poe/gapbuffer"
)

const (
	FnMessageWin  = "+poe"
	FnEmptyWin    = ""
	RuneWidthZero = '?'
)

var (
	// Main screen terminal
	screen tcell.Screen

	// menu is the main tagline above everything else
	menu *View

	// workspace contains windows.
	workspace *Workspace

	// CurWin is a pointer to the currently focused window.
	CurWin *Window

	// Channels
	events chan tcell.Event
	quit   chan bool

	// baseDir stores the dir from which the program started in
	baseDir string
)

// InitScreen initializes the tcell terminal.
func InitScreen() {
	var err error
	screen, err = tcell.NewScreen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if err = screen.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	screen.SetStyle(bodyStyle)
	screen.EnableMouse()
	screen.Clear()
	screen.Sync()
}

func InitMenu() {
	menu = &View{
		text:         &Text{buf: gapbuffer.New()},
		what:         ViewMenu,
		style:        bodyStyle,
		cursorStyle:  bodyCursorStyle,
		hilightStyle: bodyHilightStyle,
		tabstop:      4,
	}
	fmt.Fprintf(menu, "Exit New Newcol")
}

func InitWorkspace() {
	workspace = &Workspace{} // first resize event will set proper dimensions
	workspace.AddCol()
}

// LoadBuffers reads files from disk and loads them into windows. Screen need to be initialized.
func LoadBuffers(fns []string) {
	if len(fns) < 1 {
		fns = append(fns, FnEmptyWin)
	}

	// setup windows
	for i, fn := range fns {
		win := NewWindow(fn)
		win.LoadBuffer()
		if i == 0 { // first window gets focus
			CurWin = win
		}
		workspace.Col(0).AddWindow(win)
	}

	// add a base directory listing in new col
	workspace.AddCol()
	CmdOpen(baseDir)
}

func printMsg(format string, a ...interface{}) {
	// get output window
	var poewin *Window
	for _, win := range AllWindows() {
		poename := win.Dir() + string(filepath.Separator) + FnMessageWin
		poename = filepath.Clean(poename)
		if win.NameAbs() == poename && CurWin.Dir() == win.Dir() {
			poewin = win
		}
	}

	if poewin == nil {
		poewin = NewWindow(CurWin.Dir() + string(filepath.Separator) + FnMessageWin)
		poewin.body.what = ViewScratch

		if len(workspace.cols) < 2 {
			workspace.AddCol()
		}
		workspace.LastCol().AddWindow(poewin)
	}

	poewin.body.SetCursor(poewin.body.text.buf.Len(), 0)

	if a == nil {
		fmt.Fprintf(poewin.body, format)
		return
	}
	fmt.Fprintf(poewin.body, format, a...)

}

func Redraw() {
	menu.Draw()
	workspace.Draw()
	screen.Show()
}

func main() {
	flag.Parse()

	// store current working dir
	var err error
	baseDir, err = filepath.Abs(".")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	//baseDir += string(filepath.Separator)

	// Init
	InitStyles()
	InitScreen()

	// proper closing and terminal cleanup on exit and error message on a possible panic
	defer func() {
		if screen != nil {
			screen.Clear()
			screen.Fini()
		}
		if err := recover(); err != nil {
			buf := make([]byte, 1<<16)
			n := runtime.Stack(buf, true)
			fmt.Println("poe error:", err)
			fmt.Printf("%s", buf[:n+1])
			os.Exit(1)
		}
	}()

	// Setup top menu
	InitMenu()

	// Create initial workspace
	InitWorkspace()

	InitCommands()

	// This loads all buffers reading file names from command line and populates the workspace.
	LoadBuffers(flag.Args())

	events = make(chan tcell.Event, 100)
	quit = make(chan bool, 1)

	go func() {
		for {
			if screen != nil {
				events <- screen.PollEvent()
			}
		}
	}()

	// main loop
loop:
	for {
		Redraw()

		// Check for events
		var event tcell.Event
		select {
		case <-quit:
			break loop
		case event = <-events:
		}

		for event != nil {
			switch e := event.(type) {
			case *tcell.EventResize:
				w, h := screen.Size()
				menu.Resize(0, 0, w, 1)
				workspace.Resize(0, 1, w, h-1)
				screen.Clear()
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
					CurWin.HandleEvent(e)
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

				CurWin.HandleEvent(e)
			}

			event = nil

			// check tcell event queue before returning to main event check
			select {
			case event = <-events:
			default:
				event = nil
			}
		}
	}
}
