# poe

My very own [acme](http://acme.cat-v.org/)-inspired terminal text editor.

Because everything else is Not Invented Here.

![poe](./poe.png)

## Install

You need [Go](https://golang.org/). Then:

`go get -u github.com/prodhe/poe`

Binary distribution may or not be available. As of now, the source should do just fine.

## Usage

Use the mouse. `^Q` exits.

Everything is text and everything is editable. There are two ways to interact with text, `Run` or `Open`.

`Open` (Right-click or Shift+Click) will assume the selected text is a file or a directory and will open a new window listing its content. If none is found, it does nothing.

`Run` (Middle-click or Alt+Click) interprets the text as a command, which can be an internal *poe* command like `New`, `Del` or `Exit`. If none is found, it does nothing.

### Keyboard shortcuts

`^L` redraws terminal in case of rendering glitches.

`^E` moves to end of line. If already at the end, move to the beginning of next line.

`^A` move to beginning of line. If already at the beginning, move to the end of the previous line.

`^W` deletes word backwards.

`^U` deletes to beginning of line.

`^C` copies selection.

`^V` pastes into selection or place of cursor.

`^Z` undo, `^Y` redo. If you go back and change something, the future is lost. Like proper time travel.

`^S` saves current buffer to disk.

`^Q` closes whatever makes most sense to close. A window, a column or the program if nothing else remains.

### Commands

`New` opens an empty window.

`Del` closes current window. If it is the last window, the program will exit.

`Newcol` creates a new column with a new window.

`Delcol` removes current column along with all containing windows.

`Get` reloads the buffer from disk, wiping any changes you have made since the file was last read.

`Exit` closes all windows and exits the program.

Run command on `date` executes `date` as a shell command and presents its output in the message window named `+poe`. Or `pwd`, or `ls -l`, or `curl google.se`, or... you get the idea.

## Bugs

Endless. As of now, it is in constant development and things may (and will) break unannounced. Do not use for production.
