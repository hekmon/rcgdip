package gdrive

import (
	"fmt"
	"reflect"
)

const (
	stateRootFolderIDKey  = "rootFolderID"
	stateNextStartPageKey = "nextStartPage"
	stateIndexOK          = "indexOK"
)

func (c *Controller) validateState() (err error) {
	c.logger.Info("[Drive] validating local state against remote drive...")
	var (
		remoteRootID    string
		remoteRootInfos *driveFileBasicInfo
		valid           bool
	)
	// Get the current remote rootID to see if we are still accessing the same drive
	if remoteRootID, remoteRootInfos, err = c.getDriveRootFileInfo(); err != nil {
		err = fmt.Errorf("failed to get remote root drive id infos: %w", err)
		return
	}
	c.logger.Debugf("[Drive] remote root id recovered: %s", remoteRootID)
	// If the state validation failed (while not being an execution error), reset our local state
	defer func() {
		if err == nil && !valid {
			if err = c.reinitState(remoteRootID, remoteRootInfos); err != nil {
				err = fmt.Errorf("failed to reinit local state: %w", err)
			}
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
		c.logger.Notice("[Drive] no stored root folderID found: starting a new state")
		return
	}
	// Check
	if storedRootID != remoteRootID {
		c.logger.Warningf("[Drive] rootID has changed (%s -> %s): reiniting local state", storedRootID, remoteRootID)
		return
	}
	c.logger.Debug("[Drive] rootID recovered in our state matches the one upstream, checking metadata...")
	// Validate index based on root file info
	var storedRootInfo driveFileBasicInfo
	if found, err = c.index.Get(storedRootID, &storedRootInfo); err != nil {
		err = fmt.Errorf("failed to get the root folder ID infos from stored index: %w", err)
		return
	}
	if !found {
		c.logger.Warning("[Drive] we have a stored rootFolderID but it is not present in our index: reiniting local state")
		return
	}
	if !reflect.DeepEqual(storedRootInfo, *remoteRootInfos) {
		c.logger.Warningf("[Drive] our cached root property is not the same as remote (%+v -> %+v): reiniting local state",
			storedRootInfo, *remoteRootInfos)
		return
	}
	c.logger.Debugf("[Drive] the rootID '%s' and its metadata in our local state seems valid", storedRootID)
	// Do we have a next start page ?
	var nextStartPage string
	if found, err = c.state.Get(stateNextStartPageKey, &nextStartPage); err != nil {
		err = fmt.Errorf("failed to get the start page token from our local storage: %w", err)
		return
	}
	if !found {
		c.logger.Warning("[Drive] did not find any changes startNextPage token in our state: reiniting local state")
		return
	}
	// Is indexing complete ?
	if !c.state.Has(stateIndexOK) {
		c.logger.Warning("[Drive] local index is incomplete: reiniting local state")
		return
	}
	// Does the custom root folderID exists within our index ?
	if c.rc.Drive.Options.RootFolderID != "" && !c.index.Has(c.rc.Drive.Options.RootFolderID) {
		c.logger.Warningf("[Drive] custom root folder ID ('%s') not found within our index: reiniting local state", c.rc.Drive.Options.RootFolderID)
		return
	}
	// All good
	valid = true
	return
}

func (c *Controller) reinitState(remoteRootID string, remoteRootInfos *driveFileBasicInfo) (err error) {
	// Clear state and index
	if err = c.state.Clear(); err != nil {
		err = fmt.Errorf("failed to clean the state: %w", err)
		return
	}
	if err = c.index.Clear(); err != nil {
		err = fmt.Errorf("failed to clean the index: %w", err)
		return
	}
	// Store the root folder ID within the state
	if err = c.state.Set(stateRootFolderIDKey, remoteRootID); err != nil {
		err = fmt.Errorf("failed to save root folder fileID within the local state: %w", err)
		return
	}
	// Insert the first index item: root folder
	if err = c.index.Set(remoteRootID, remoteRootInfos); err != nil {
		err = fmt.Errorf("failed to save root folder file infos within the local index: %w", err)
		return
	}
	// Special case for team drives, the root folderID can have a different form
	if c.rc.Drive.Options.TeamDriveID != "" && remoteRootID != c.rc.Drive.Options.TeamDriveID {
		c.logger.Debugf("[Drive] retreived root folderID '%s' is different than supplied teamdrive ID '%s': cloning it within the index",
			remoteRootID, c.rc.Drive.Options.TeamDriveID)
		if err = c.index.Set(c.rc.Drive.Options.TeamDriveID, remoteRootInfos); err != nil {
			err = fmt.Errorf("failed to clone root folder file infos as teamdrive within the local index: %w", err)
			return
		}
	}
	// Does the custom root folderID exists upstream ?
	if c.rc.Drive.Options.RootFolderID != "" {
		if _, err = c.getDriveFileInfo(c.rc.Drive.Options.RootFolderID); err != nil {
			err = fmt.Errorf("failed to validate rclone declared custom root folder ID upstream: %w", err)
			return
		}
	}
	// Get changes starting point
	var nextStartPage string
	if nextStartPage, err = c.getDriveChangesStartPage(); err != nil {
		err = fmt.Errorf("failed to get the start page token from Drive API: %w", err)
		return
	}
	if err = c.state.Set(stateNextStartPageKey, nextStartPage); err != nil {
		err = fmt.Errorf("failed to save the startPageToken within our state: %w", err)
		return
	}
	// Index all the things
	if err = c.initialIndexBuild(); err != nil {
		err = fmt.Errorf("failed to index the drive: %w", err)
		return
	}
	if err = c.state.Set(stateIndexOK, true); err != nil {
		err = fmt.Errorf("failed to save the startPageToken within our state: %w", err)
		return
	}
	return
}
