package gdrivewatcher

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

const (
	maxChangesPerPage = 1000
)

func (c *Controller) getChangesStartPage() (err error) {
	// Get start page token
	changesReq := c.driveClient.Changes.GetStartPageToken().Context(c.ctx)
	if c.rc.Drive.TeamDrive != "" {
		changesReq.SupportsAllDrives(true).DriveId(c.rc.Drive.TeamDrive)
	}
	changesStart, err := changesReq.Do()
	if err != nil {
		return
	}
	// Save it
	if err = c.state.Set(stateNextStartPageKey, changesStart.StartPageToken); err != nil {
		err = fmt.Errorf("failed to save the startPageToken within our state: %w", err)
		return
	}
	return
}

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
	changes, err := c.fetchChanges(backupStartToken)
	if err != nil {
		err = fmt.Errorf("failed to get all changes recursively: %w", err)
		return
	}
	c.logger.Debugf("[DriveWatcher] %d raw change(s) recovered in %v", len(changes), time.Since(start))
	// Build the index with parents for further path computation
	indexStart := time.Now()
	if err = c.incorporateChangesToIndex(changes); err != nil {
		err = fmt.Errorf("failed to build up the parent index for the %d changes retreived: %w", len(changes), err)
		return
	}
	if c.logger.IsDebugShown() && len(changes) > 0 {
		// NbKeys has a performance hit
		c.logger.Debugf("[DriveWatcher] index updated in %v, currently containing %d nodes", time.Since(indexStart), c.index.NbKeys())
	}
	// Process each event
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
	// Done
	c.logger.Infof("[DriveWatcher] %d valid change(s) on %d recovered change(s) compiled in %v", len(changedFiles), len(changes), time.Since(start))
	return
}

func (c *Controller) fetchChanges(nextPageToken string) (changes []*drive.Change, err error) {
	c.logger.Debug("[DriveWatcher] getting a new page of changes...")
	// Build Request
	changesReq := c.driveClient.Changes.List(nextPageToken).Context(c.ctx)
	changesReq.IncludeRemoved(true)
	if c.rc.Drive.TeamDrive != "" {
		changesReq.SupportsAllDrives(true).IncludeItemsFromAllDrives(true).DriveId(c.rc.Drive.TeamDrive)
	}
	{
		// // Dev
		// changesReq.PageSize(1)
		// changesReq.Fields(googleapi.Field("*"))
	}
	{
		// Prod
		changesReq.PageSize(maxChangesPerPage)
		changesReq.Fields(googleapi.Field("nextPageToken"), googleapi.Field("newStartPageToken"),
			googleapi.Field("changes"), googleapi.Field("changes/fileId"), googleapi.Field("changes/removed"),
			googleapi.Field("changes/time"), googleapi.Field("changes/changeType"), googleapi.Field("changes/file"),
			googleapi.Field("changes/file/name"), googleapi.Field("changes/file/mimeType"), googleapi.Field("changes/file/trashed"),
			googleapi.Field("changes/file/parents"), googleapi.Field("changes/file/createdTime"))
	}
	// Execute Request
	if err = c.limiter.Wait(c.ctx); err != nil {
		err = fmt.Errorf("can not execute API request, waiting for the limiter failed: %w", err)
		return
	}
	start := time.Now()
	changeList, err := changesReq.Do()
	if err != nil {
		err = fmt.Errorf("failed to execute the API query for changes list: %w", err)
		return
	}
	c.logger.Debugf("[DriveWatcher] changes page obtained in %v", time.Since(start))
	// Extract changes from answer
	changes = changeList.Changes
	// Is there any pages left ?
	if changeList.NextPageToken != "" {
		c.logger.Debugf("[DriveWatcher] another page of changes is available at %s", changeList.NextPageToken)
		var nextPagesChanges []*drive.Change
		if nextPagesChanges, err = c.fetchChanges(changeList.NextPageToken); err != nil {
			err = fmt.Errorf("failed to get change list next page: %w", err)
			return
		}
		changes = append(changes, nextPagesChanges...)
		return
	}
	// We are the last page of results, save token for next run
	if changeList.NewStartPageToken == "" {
		err = errors.New("end of changelist should contain NewStartPageToken")
		return
	}
	c.logger.Debugf("[DriveWatcher] no more changes pages, recovering the marker for next run: %s", changeList.NewStartPageToken)
	// save new start token for next run
	if err = c.state.Set(stateNextStartPageKey, changeList.NewStartPageToken); err != nil {
		err = fmt.Errorf("failed to save the nextStartPageToken within local state: %w", err)
		return
	}
	// Done
	if c.logger.IsDebugShown() {
		for index, change := range changes {
			c.logger.Debugf("[DriveWatcher] raw change #%d: %+v", index+1, *change)
		}
	}
	return
}

func (c *Controller) incorporateChangesToIndex(changes []*drive.Change) (err error) {
	c.logger.Debugf("[DriveWatcher] update the index using %d change(s)", len(changes))
	// Build the file index starting by infos contained in the change list
	lookup := make([]string, 0, len(changes))
	for _, change := range changes {
		// Skip is the change is drive metadata related
		if change.ChangeType != "file" {
			continue
		}
		// If file deleted, we won't have any information. To make it valid, we must have it within our index
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
	if err = c.fetchIfMissing(lookup); err != nil {
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
	if change.Removed {
		defer func() {
			if deleteErr := c.index.Delete(change.FileId); err != nil {
				c.logger.Errorf("[DriveWatcher] failed to delete fileID '%s' from local index after processing its removed change event: %s",
					change.FileId, deleteErr)
			} else {
				c.logger.Debugf("[DriveWatcher] deletes fileID '%s' from local index after processing its removed change event",
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
	// Save up the consolidated info for return collection
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
			c.logger.Warningf("[DriveWatcher] fileID %s has been removed but it is not within our index: we can not compute its path and therefor will be skipped",
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
