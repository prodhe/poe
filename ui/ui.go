package ui

import (
	"github.com/prodhe/poe/editor"
	uitcell "github.com/prodhe/poe/ui/tcell"
)

const (
	SignalQuit int = iota
)

type Interface interface {
	// Init initializes the user interface.
	Init(ed editor.Editor) error

	// Close will close and clean up any resources held by the UI.
	Close()

	// Listen loops for events and acts upon the editor as it sees fit. It is up to the implementation to decide what events it will look for and how to handle them. This could for example be keyboard input in a terminal implementation updating the buffers or an HTTP server modifing the buffers remotely, depending on the implementation of the UI.
	Listen()
}

func NewTcell() Interface {
	return &uitcell.Tcell{}
}

func NewCli() Interface {
	return &Cli{}
}
