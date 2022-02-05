package gdrivewatcher

import (
	"errors"
	"fmt"
	"reflect"
)

const (
	stateNextStartPageKey = "nextStartPage"
	stateRootFolderIDKey  = "rootFolderID"
)

func (c *Controller) validateStateAgainstRemoteDrive() (err error) {
	c.logger.Info("[DriveWatcher] validating local state against remote drive...")
	var (
		valid           bool
		remoteRootID    string
		remoteRootInfos *driveFileBasicInfo
	)
	// Get the current remote rootID to see if we are still accessing the same drive
	if remoteRootID, remoteRootInfos, err = c.getDriveRootFileInfo(); err != nil {
		err = fmt.Errorf("failed to get remote root drive id infos: %w", err)
		return
	}
	// If the remote drive does not validate, invalid our local state
	defer func() {
		if !valid {
			// Clear state and index
			if err = c.state.Clear(); err != nil {
				err = fmt.Errorf("failed to clean the state: %w", err)
				return
			}
			if err = c.index.Clear(); err != nil {
				err = fmt.Errorf("failed to clean the index: %w", err)
				return
			}
			// Insert the first index item: root folder
			if err = c.index.Set(remoteRootID, remoteRootInfos); err != nil {
				err = fmt.Errorf("failed to save root folder file infos within the local index: %w", err)
				return
			}
			// Store the root folder ID within the state
			if err = c.state.Set(stateRootFolderIDKey, remoteRootID); err != nil {
				err = fmt.Errorf("failed to save root folder fileID within the local state: %w", err)
				return
			}
		} else {
			c.logger.Info("[DriveWatcher] local state seems valid")
		}
	}()
	// First do we have a stored rootID ?
	var (
		storedRootID string
		found        bool
	)
	if found, err = c.state.Get(stateRootFolderIDKey, &storedRootID); err != nil {
		err = fmt.Errorf("failed to get the root folder ID from stored state: %w", err)
		return
	}
	if !found {
		c.logger.Info("[DriveWatcher] no stored root folderID found, starting a new state")
		return
	}
	// Check
	if storedRootID != remoteRootID {
		c.logger.Warningf("[DriveWatcher] rootID has changed (%s -> %s), invalidating state", storedRootID, remoteRootID)
		return
	}
	// Validate index based on root file info
	var storedRootInfo driveFileBasicInfo
	if found, err = c.index.Get(storedRootID, &storedRootInfo); err != nil {
		err = fmt.Errorf("failed to get the root folder ID infos from stored index: %w", err)
		return
	}
	if !found {
		c.logger.Warning("[DriveWatcher] we have a stored rootFolderID but it is not present in our index, invalidating state")
		return
	}
	if !reflect.DeepEqual(storedRootInfo, *remoteRootInfos) {
		c.logger.Warningf("[DriveWatcher] our cached root property is not the same as remote, invalidating state: %+v -> %+v",
			storedRootInfo, *remoteRootInfos)
		return
	}
	// All good
	c.logger.Debugf("[DriveWatcher] the root folderID '%s' in our local state seems valid", storedRootID)
	valid = true
	return
}

func (c *Controller) populate() (err error) {
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
	nbKeys := c.index.NbKeys()
	switch nbKeys {
	case 0:
		return errors.New("local index contains 0 keys: initialization must has gone wrong as at least 1 key (root folder) should be present")
	case 1:
		if err = c.initialIndexBuild(); err != nil {
			err = fmt.Errorf("failed to index the drive: %w", err)
			return
		}
	default:
		c.logger.Debugf("[DriveWatcher] local index contains %d nodes", nbKeys)
	}
	return
}
