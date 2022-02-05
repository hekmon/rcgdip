package gdrivewatcher

import (
	"fmt"
	"time"

	"google.golang.org/api/drive/v3"
)

type fileChange struct {
	Event   time.Time
	Folder  bool
	Deleted bool
	Paths   []string
}

func (c *Controller) getFilesChanges() (changedFiles []fileChange, err error) {
	// Save the start token in case something goes wrong for future retry
	var (
		backupStartToken string
		found            bool
	)
	if found, err = c.state.Get(stateNextStartPageKey, &backupStartToken); err != nil {
		err = fmt.Errorf("failed to get the start page token from stored state: %w", err)
		return
	}
	if !found {
		err = fmt.Errorf("start page token not found within stored state: %w", err)
		return
	}
	defer func() {
		if err != nil {
			if setErr := c.state.Set(stateNextStartPageKey, backupStartToken); setErr != nil {
				err = fmt.Errorf("2 errors: %w | failed to restore the startPageToken within our state: %s", err, setErr)
				return
			}
		}
	}()
	// Get changes
	start := time.Now()
	changes, nextStartPage, err := c.getDriveChanges(backupStartToken)
	if err != nil {
		err = fmt.Errorf("failed to get all changes recursively: %w", err)
		return
	}
	if nextStartPage != backupStartToken {
		// if no changes, token stays the same
		if err = c.state.Set(stateNextStartPageKey, nextStartPage); err != nil {
			err = fmt.Errorf("failed to save the nextStartPageToken within local state: %w", err)
			return
		}
	}
	c.logger.Debugf("[DriveWatcher] %d raw change(s) recovered in %v", len(changes), time.Since(start))
	if len(changes) == 0 {
		c.logger.Info("[DriveWatcher] no changes detected")
		return
	}
	// Build the index with parents for further path computation
	indexStart := time.Now()
	if err = c.addChangesFilesToIndex(changes); err != nil {
		err = fmt.Errorf("failed to build up the parent index for the %d changes retreived: %w", len(changes), err)
		return
	}
	if c.logger.IsDebugShown() {
		// NbKeys has a performance hit, call it only if we need to
		c.logger.Debugf("[DriveWatcher] index updated in %v, currently containing %d nodes", time.Since(indexStart), c.index.NbKeys())
	}
	// Process each event
	processStart := time.Now()
	changedFiles = make([]fileChange, 0, len(changes))
	var fc *fileChange
	for _, change := range changes {
		// Transforme change into a suitable file event
		if fc, err = c.processChange(change); err != nil {
			err = fmt.Errorf("failed to process the %d changes retreived: %w", len(changes), err)
			return
		}
		// If change is valid, add it to the return list
		if fc != nil {
			changedFiles = append(changedFiles, *fc)
		}
	}
	if len(changedFiles) != len(changes) {
		c.logger.Debugf("[DriveWatcher] filtered out %d change(s) that was not a file change", len(changes)-len(changedFiles))
	}
	c.logger.Debugf("[DriveWatcher] %d raw change(s) processed in %v", len(changes), time.Since(processStart))
	// Done
	c.logger.Infof("[DriveWatcher] %d valid change(s) on %d recovered change(s) compiled in %v", len(changedFiles), len(changes), time.Since(start))
	return
}

func (c *Controller) addChangesFilesToIndex(changes []*drive.Change) (err error) {
	c.logger.Debugf("[DriveWatcher] update the index using %d change(s)", len(changes))
	// Build the file index starting by infos contained in the change list
	lookup := make([]string, 0, len(changes))
	for _, change := range changes {
		// Skip is the change is drive metadata related
		if change.ChangeType != "file" {
			continue
		}
		// If file deleted, let's keep it in the index to rebuild its path, it will be deleted at the end of the process
		if change.Removed {
			continue
		}
		// Sometimes the file field come back empty, no idea why
		if change.File == nil {
			// should not happen if not remove change event
			c.logger.Warningf("[DriveWatcher] file change for fileID %s had its file metadata empty, adding it to the lookup list", change.FileId)
			continue
		}
		// Update index with infos
		if err = c.index.Set(change.FileId, driveFileBasicInfo{
			Name:    change.File.Name,
			Folder:  change.File.MimeType == folderMimeType,
			Parents: change.File.Parents,
		}); err != nil {
			err = fmt.Errorf("failed to saved fileID '%s' within the local index: %w", change.FileId, err)
			return
		}
		// Add its parents for search
		var found bool
		for _, parentID := range change.File.Parents {
			// add parent to lookup if not already present in changes
			found = false
			for _, changeCheck := range changes {
				if changeCheck.FileId == parentID {
					found = true
					break
				}
			}
			if !found {
				lookup = append(lookup, parentID)
			}
		}
	}
	// Found out all missing parents infos
	if err = c.fetchAndAddToIndexIfMissing(lookup); err != nil {
		err = fmt.Errorf("failed to recover all parents files infos: %w", err)
		return
	}
	// Done
	return
}

func (c *Controller) processChange(change *drive.Change) (fc *fileChange, err error) {
	// Skip if the change is drive metadata related
	if change.ChangeType != "file" {
		return
	}
	// In case the file metadata was not provided within the change, extract info from our index (main case: removal)
	fileName, fileIsFolder, fileTrashed, skip, err := c.compileFileInfosFor(change)
	if err != nil {
		err = fmt.Errorf("failed to compile file info: %w", err)
		return
	}
	if skip {
		return
	}
	// Purge file from index at the end if deletion
	if change.Removed || fileTrashed {
		defer func() {
			if deleteErr := c.index.Delete(change.FileId); err != nil {
				c.logger.Errorf("[DriveWatcher] failed to delete fileID '%s' from local index after processing its removed change event: %s",
					change.FileId, deleteErr)
			} else {
				c.logger.Debugf("[DriveWatcher] deleted fileID '%s' from local index after processing its removed/trashed change event",
					change.FileId)
			}
		}()
	}
	// Compute possible paths (bottom up)
	reversedPaths, err := c.generateReversePaths(change.FileId)
	if err != nil {
		err = fmt.Errorf("failed to generate path for fileID %s, name '%s': %w", change.FileId, fileName, err)
		return
	}
	// Validate and reverse the paths (from bottom up to top down) to be exploitables
	validPaths := make([]string, 0, len(reversedPaths))
	for _, reversedPath := range reversedPaths {
		// If custom root folder id, search it and rewrite paths with new root
		if c.rc.Drive.RootFolderID != "" {
			// TODO: cutAt yield new driveFilePath
			if !reversedPath.CutAt(c.rc.Drive.RootFolderID) {
				c.logger.Debugf("[DriveWatcher] path '%s' does not contain the custom root folder id, discarding it", reversedPath.Reverse().Path())
				continue // root folder id not found in this path, skipping
			}
		}
		// Path valid, adding it to the list
		validPaths = append(validPaths, reversedPath.Reverse().Path())
	}
	if len(validPaths) == 0 {
		// no valid path found (because of root folder id) skipping this change
		c.logger.Debugf("[DriveWatcher] change for file '%s' does not contain any valid path, discarding it", fileName)
		return
	}
	// Convert times
	changeTime, err := time.Parse(time.RFC3339, change.Time)
	if err != nil {
		err = fmt.Errorf("failed to convert change time for fileID %s, name '%s': %w", change.FileId, fileName, err)
		return
	}
	// Return the consolidated info for caller
	fc = &fileChange{
		Event:   changeTime,
		Folder:  fileIsFolder,
		Deleted: change.Removed || fileTrashed,
		Paths:   validPaths,
	}
	return
}

func (c *Controller) compileFileInfosFor(change *drive.Change) (fileName string, fileIsFolder bool, fileTrashed bool, skip bool, err error) {
	// If file metadata is attached to change event, use them directly
	if change.File != nil {
		fileName = change.File.Name
		fileIsFolder = change.File.MimeType == folderMimeType
		fileTrashed = change.File.Trashed
		return
	}
	// Else, search it within our local index
	var (
		fi    driveFileBasicInfo
		found bool
	)
	if found, err = c.index.Get(change.FileId, &fi); err != nil {
		err = fmt.Errorf("failed to get fileID '%s' infos from local index: %w", change.FileId, err)
		return
	}
	if !found {
		if change.Removed {
			c.logger.Debugf("[DriveWatcher] fileID %s has been removed but it is not within our index: we can not compute its path and therefor will be skipped",
				change.FileId)
			skip = true
		} else {
			err = fmt.Errorf("change does not contain file metadata and its fileID '%s' was not found within the index", change.FileId)
		}
		return
	}
	fileName = fi.Name
	fileIsFolder = fi.Folder
	return
}
