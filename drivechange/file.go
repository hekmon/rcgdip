package drivechange

import "time"

type File struct {
	Event   time.Time
	Folder  bool
	Deleted bool
	Paths   []string
}
