package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/prodhe/poe/editor"
	"github.com/prodhe/poe/ui"
)

const poeVersion = "0.0.0"

func main() {
	version := flag.Bool("v", false, "prints current version of poe")
	cli := flag.Bool("c", false, "prints current version of poe")

	flag.Parse()

	if *version {
		fmt.Println(poeVersion)
		os.Exit(0)
	}

	// new editor with loaded files
	e := editor.New()
	e.LoadBuffers(flag.Args())

	// load client user interface
	cui := ui.NewTcell()
	if *cli {
		cui = ui.NewCli()
	}

	cui.Init(e)

	// close ui and show stack trace on panic
	defer func() {
		cui.Close()

		if err := recover(); err != nil {
			buf := make([]byte, 1<<16)
			n := runtime.Stack(buf, true)
			fmt.Println("poe error:", err)
			fmt.Printf("%s", buf[:n+1])
			os.Exit(1)
		}
	}()

	// This will loop and listen on chosen UI.
	cui.Listen()
}
