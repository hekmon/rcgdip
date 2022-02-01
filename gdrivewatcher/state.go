package gdrivewatcher

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/hekmon/rcgdip/diskstate"
)

const (
	stateFileName = "drivewatcher_state.json"
)

type stateFile struct {
	StartPage string     `json:"changes_start_page"`
	Index     filesIndex `json:"remote_files_index"`
}

func (c *Controller) restoreState() (err error) {
	c.logger.Info("[DriveWatcher] restoring state...")
	var recoveredState stateFile
	// Load from file
	if err = diskstate.LoadJSON(stateFileName, &recoveredState); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("failed to load state from disk: %w", err)
			return
		}
		// First run
		err = nil
		c.index = nil
		c.logger.Info("[DriveWatcher] starting from an empty state")
		return
	}
	// Extract and inject
	c.indexAccess.Lock()
	c.index = recoveredState.Index
	c.indexAccess.Unlock()
	c.startPageToken = recoveredState.StartPage
	// Done
	c.logger.Debugf("[DriveWatcher] index lodaded from disk containing %d nodes", len(c.index))
	return
}

func (c *Controller) SaveState() (err error) {
	c.logger.Info("[DriveWatcher] saving state...")
	// Build the state file
	c.indexAccess.RLock()
	defer c.indexAccess.RUnlock()
	state := stateFile{
		StartPage: c.startPageToken,
		Index:     c.index,
	}
	// Dump it to disk
	if err = diskstate.SaveJSON(stateFileName, state, c.logger.IsDebugShown()); err != nil {
		err = fmt.Errorf("failed to dump state to disk: %w", err)
		return
	}
	c.logger.Debugf("[DriveWatcher] saved %d index nodes to disk", len(c.index))
	return
}
