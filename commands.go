package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type CommandFunc func(args string)

var poecmds map[string]CommandFunc

func InitCommands() {
	poecmds = map[string]CommandFunc{
		"Exit":   CmdExit,
		"New":    CmdNew,
		"Del":    CmdDel,
		"Edit":   CmdEdit,
		"Newcol": CmdNewcol,
	}
}

func RunCommand(input string) {
	if input == "" {
		return
	}

	input = strings.Trim(input, "\t\n ")

	// check poe default commands
	cmd := strings.Split(string(input), " ")
	if fn, ok := poecmds[cmd[0]]; ok {
		fn(strings.TrimPrefix(input, cmd[0]))
		return
	}

	// Edit shortcuts for external commands and piping
	switch input[0] {
	case '!', '<', '>', '|':
		CmdEdit(input)
	}

	CmdEdit("!" + input)
}

func CmdExit(args string) {
	ok := true
	for _, win := range AllWindows() {
		if !win.CanClose() {
			ok = false
		}
	}
	if ok {
		quit <- true
	}
}

func CmdNewcol(args string) {
	screen.Clear()
	screen.Sync()
	workspace.AddCol()
	CmdNew("")
}

func CmdNew(args string) {
	screen.Clear()
	win := NewWindow(FnEmptyWin)
	workspace.LastCol().AddWindow(win)
}

func CmdOpen(fn string) {
	screen.Clear()
	var win *Window
	win = FindWindow(fn)
	if win == nil { // only load windows that do no already exists
		win := NewWindow(fn)
		workspace.LastCol().AddWindow(win)
		win.LoadBuffer()
	}
}

func CmdDel(args string) {
	CurWin.Close()
}

func CmdEdit(args string) {
	if len(args) < 2 {
		return
	}

	switch args[0] {
	case 'f':
		var names []string
		for _, win := range AllWindows() {
			names = append(names, fmt.Sprintf("  %3s %s", win.Flags(), win.Name()))
		}
		printMsg("buffers:\n%s\n", strings.Join(names, "\n"))
	case '!':
		os.Chdir(CurWin.Dir())
		cmd := strings.Split(string(args[1:]), " ")
		path, err := exec.LookPath(cmd[0])
		if err != nil { // path not found, break with silence
			//printMsg("path not found: %s\n", cmd[0])
			break
		}
		out, err := exec.Command(path, cmd[1:]...).Output()
		if err != nil {
			printMsg("error: %s\n", err)
			break
		}
		// if command produced output, print it
		outstr := string(out)
		if outstr != "" {
			printMsg("%s", outstr)
		}
	default:
		printMsg("?\n")
	}
}
