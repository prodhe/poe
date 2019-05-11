package uitcell

import "github.com/gdamore/tcell"

var (
	// body is the main editing buffer
	bodyStyle        tcell.Style
	bodyCursorStyle  tcell.Style
	bodyHilightStyle tcell.Style

	// tag is the window tag line above the body
	tagStyle               tcell.Style
	tagCursorStyle         tcell.Style
	tagHilightStyle        tcell.Style
	tagSquareStyle         tcell.Style
	tagSquareModifiedStyle tcell.Style

	// vertline is the vertical line separating columns
	vertlineStyle tcell.Style

	// unprintable rune
	unprintableStyle tcell.Style
)

// initStyles initializes the different styles (colors for background/foreground).
func initStyles() error {
	//bodyStyle = tcell.StyleDefault

	bodyStyle = tcell.StyleDefault.
		Background(tcell.NewHexColor(0xffffea)).
		Foreground(tcell.ColorBlack)

	bodyCursorStyle = bodyStyle.
		Background(tcell.NewHexColor(0xeaea9e))

	//bodyHilightStyle = bodyStyle.Reverse(true)
	bodyHilightStyle = bodyStyle.
		Background(tcell.NewHexColor(0xa6a65a))
	unprintableStyle = bodyStyle.
		Foreground(tcell.ColorRed)

	//tagStyle = tcell.StyleDefault.Reverse(true)
	tagStyle = tcell.StyleDefault.
		Background(tcell.NewHexColor(0xeaffff)).
		Foreground(tcell.ColorBlack)
	tagCursorStyle = tagStyle.
		Background(tcell.NewHexColor(0x8888cc)).
		Foreground(tcell.ColorBlack)
	tagHilightStyle = tagStyle.
		Background(tcell.NewHexColor(0x8888cc))
	tagSquareStyle = tagStyle.
		Background(tcell.NewHexColor(0x8888cc))
	tagSquareModifiedStyle = tagStyle.
		Background(tcell.NewHexColor(0x2222cc))

	vertlineStyle = bodyStyle.Reverse(false)

	return nil
}
