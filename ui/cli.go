package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/prodhe/poe/editor"
)

// Cli implements ui.Interface with simple command-line driven user actions.
type Cli struct {
	ed editor.Editor
}

func (c *Cli) Init(e editor.Editor) error {
	c.ed = e
	return nil
}

func (c *Cli) Close() {
}

func (c *Cli) Listen() {
	scanner := bufio.NewScanner(os.Stdin)
	var input []string
outer:
	for {
		scanner.Scan()
		input = strings.Split(scanner.Text(), " ")
		switch input[0][0] { // first rune in input
		case 'q': // quit
			break outer
		case 'f':
			ids, bs := c.ed.Buffers()
			fmt.Printf("%v\n%v\n", ids, bs)
		case 'b':
			if len(input) > 1 {
				id, _ := strconv.ParseInt(input[1], 10, 64)
				c.ed.Buffer(id)
				break
			}
			c.ed.NewBuffer()
		case 'a':
			if len(input) < 2 {
				break
			}
			//c.ed.Current().Write([]byte(strings.Join(input[1:], " ")))
		default:
			fmt.Println("?")
		}
	}
}
