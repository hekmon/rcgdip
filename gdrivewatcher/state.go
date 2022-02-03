package gdrivewatcher

import (
	"fmt"
	"reflect"
)

const (
	stateNextStartPageKey = "nextStartPage"
	stateRootFolderIDKey  = "rootFolderID"
)

func (c *Controller) validateState() (err error) {
	c.logger.Info("[DriveWatcher] validating state...")
	var valid bool
	// If the remote drive does not validate, invalid our local state
	defer func() {
		if !valid {
			if err = c.state.Clear(); err != nil {
				err = fmt.Errorf("failed to clean the state: %w", err)
				return
			}
			if err = c.index.Clear(); err != nil {
				err = fmt.Errorf("failed to clean the index: %w", err)
				return
			}
		}
	}()
	// First do we have one ?
	var (
		storedRootID string
		found        bool
	)
	if found, err = c.state.Get(stateRootFolderIDKey, &storedRootID); err != nil {
		err = fmt.Errorf("failed to get the root folder ID from stored state: %w", err)
		return
	}
	if !found {
		c.logger.Info("[DriveWatcher] no root folderID found, starting a new state")
		return
	}
	// Get the current remote rootID to see if we are still accessing the same drive
	remoteRootID, rootInfos, err := c.getRootInfo()
	if err != nil {
		err = fmt.Errorf("failed to get remote root drive id infos: %w", err)
		return
	}
	// Check
	if storedRootID != remoteRootID {
		c.logger.Warningf("[DriveWatcher] rootID has changed (%s -> %s), invalidating state", storedRootID, remoteRootID)
		return
	}
	// Validate index
	var storedRootInfo driveFileBasicInfo
	if found, err = c.index.Get(storedRootID, &storedRootInfo); err != nil {
		err = fmt.Errorf("failed to get the root folder ID infos from stored index: %w", err)
		return
	}
	if !found {
		c.logger.Warning("[DriveWatcher] we have a stored rootFolderID but it is not present in our index, invalidating state")
		return
	}
	if !reflect.DeepEqual(storedRootInfo, rootInfos) {
		c.logger.Warningf("[DriveWatcher] our cached root property is not the same as remote, invalidating state: %+v -> %+v",
			storedRootInfo, rootInfos)
		return
	}
	// All good
	c.logger.Debugf("[DriveWatcher] the root folderID '%s' in our local state seems valid", storedRootID)
	valid = true
	return
}

func (c *Controller) initState() (err error) {
	var found bool
	// StartNextPage
	var nextStartPage string
	if found, err = c.state.Get(stateNextStartPageKey, &nextStartPage); err != nil {
		err = fmt.Errorf("failed to get the start page token from our local storage: %w", err)
		return
	}
	if !found {
		if err = c.getChangesStartPage(); err != nil {
			err = fmt.Errorf("failed to get the start page token from Drive API: %w", err)
			return
		}
	}
	// Index
	if nbKeys := c.index.NbKeys(); nbKeys == 0 {
		var rootFolderID string
		// Populate the index
		if rootFolderID, err = c.initialIndexBuild(); err != nil {
			err = fmt.Errorf("failed to index the drive: %w", err)
			return
		}
		// Save the rootfolder id within our state
		if err = c.state.Set(stateRootFolderIDKey, rootFolderID); err != nil {
			err = fmt.Errorf("failed to save the drive root folder id: %w", err)
			return
		}
	}
	return
}
