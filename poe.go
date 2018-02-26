package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/prodhe/poe/editor"
	"github.com/prodhe/poe/ui"
)

func main() {
	flag.Parse()

	e := editor.New()

	e.LoadBuffers(flag.Args())

	ui := ui.NewTcell()
	//ui := ui.NewCli()
	ui.Init(e)

	// close ui and show stack trace on panic
	defer func() {
		ui.Close()

		if err := recover(); err != nil {
			buf := make([]byte, 1<<16)
			n := runtime.Stack(buf, true)
			fmt.Println("poe error:", err)
			fmt.Printf("%s", buf[:n+1])
			os.Exit(1)
		}
	}()

	// This will loop and listen on chosen UI.
	ui.Listen()
}
