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

type stateData struct {
	RootID         string     `json:"root_check"`
	StartPageToken string     `json:"changes_start_page"`
	Index          filesIndex `json:"remote_files_index"`
}

func (c *Controller) restoreState() (err error) {
	c.logger.Info("[DriveWatcher] restoring state...")
	var recoveredState stateData
	// Load from file
	if err = diskstate.LoadJSON(stateFileName, &recoveredState); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("failed to load state from disk: %w", err)
			return
		}
		// First run
		err = nil
		c.state.Index = nil // mark as non initialized
		return
	}
	// Extract and inject (no need to use mutexes here as we are in the init phase)
	c.state.RootID = recoveredState.RootID
	c.state.StartPageToken = recoveredState.StartPageToken
	c.state.Index = recoveredState.Index
	// Done
	c.logger.Debugf("[DriveWatcher] index lodaded from disk containing %d nodes", len(c.state.Index))
	return
}

func (c *Controller) SaveState() (err error) {
	c.logger.Info("[DriveWatcher] saving state...")
	// Build the state file (lock the mutex in case the work is running a batch)
	defer c.stateAccess.Unlock()
	c.stateAccess.Lock()
	state := stateData{
		RootID:         c.state.RootID,
		StartPageToken: c.state.StartPageToken,
		Index:          c.state.Index,
	}
	// Dump it to disk
	if err = diskstate.SaveJSON(stateFileName, state, c.logger.IsDebugShown()); err != nil {
		err = fmt.Errorf("failed to dump state to disk: %w", err)
		return
	}
	c.logger.Debugf("[DriveWatcher] saved %d index nodes to disk", len(c.state.Index))
	return
}
