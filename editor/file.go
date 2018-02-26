package editor

import (
	"time"
)

// File holds information about a file on disk.
type File struct {
	name   string
	read   bool      // true if file has been read
	mtime  time.Time // of file when last read/written
	sha256 string    // of file when last read/written
}
