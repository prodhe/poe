package uitcell

import tcell "github.com/gdamore/tcell/v2"

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
	bodyStyle = tcell.StyleDefault
	// bodyCursorStyle = tcell.StyleDefault
	bodyHilightStyle = bodyStyle.Reverse(true)

	tagStyle = tcell.StyleDefault.Reverse(true)
	tagCursorStyle = tcell.StyleDefault.Reverse(true)
	tagHilightStyle = tagStyle.Reverse(false)
	tagSquareStyle = tcell.StyleDefault.Reverse(true)
	tagSquareModifiedStyle = tcell.StyleDefault.Reverse(true)

	vertlineStyle = bodyStyle.Reverse(false)
	unprintableStyle = bodyStyle.
		Foreground(tcell.ColorRed)
	return nil
}

// acmeStyles sets the color scheme to acme.
func setStyleAcme() error {
	bodyStyle = tcell.StyleDefault.
		Background(tcell.NewHexColor(0xffffea)).
		Foreground(tcell.ColorBlack.TrueColor())
	// bodyCursorStyle = bodyStyle.Background(tcell.NewHexColor(0xeaea9e))
	bodyHilightStyle = bodyStyle.
		Background(tcell.NewHexColor(0xeeee9e))
	unprintableStyle = bodyStyle.
		Foreground(tcell.ColorRed.TrueColor())
	tagStyle = tcell.StyleDefault.
		Background(tcell.NewHexColor(0xeaffff)).
		Foreground(tcell.ColorBlack.TrueColor())
	// tagCursorStyle = tagStyle.Background(tcell.NewHexColor(0x8888cc)).Foreground(tcell.ColorBlack.TrueColor())
	tagHilightStyle = tagStyle.
		Background(tcell.NewHexColor(0x9eeeee))
	tagSquareStyle = tagStyle.
		Background(tcell.NewHexColor(0xeaffff))
	tagSquareModifiedStyle = tagStyle.
		Background(tcell.NewHexColor(0x000099))
	vertlineStyle = bodyStyle.Reverse(false)

	return nil
}
