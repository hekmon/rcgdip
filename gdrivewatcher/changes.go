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
	folderMimeType    = "application/vnd.google-apps.folder"
)

type fileChange struct {
	Event   time.Time
	Folder  bool
	Deleted bool
	Paths   []string
	Created time.Time
}

func (c *Controller) GetFilesChanges() (changedFiles []fileChange, err error) {
	// Save the start token in case something goes wrong for future retry
	backupStartToken := c.startPageToken
	defer func() {
		if err != nil {
			c.startPageToken = backupStartToken
		}
	}()
	// Get changes
	start := time.Now()
	changes, err := c.fetchChanges(c.startPageToken)
	if err != nil {
		err = fmt.Errorf("failed to get all changes recursively: %w", err)
		return
	}
	c.logger.Debugf("[DriveWatcher] %d raw change(s) recovered in %v", len(changes), time.Since(start))
	// Build the index with parents for further path computation
	indexStart := time.Now()
	if err = c.incorporateChanges(changes); err != nil {
		err = fmt.Errorf("failed to build up the parent index for the %d changes retreived: %w", len(changes), err)
		return
	}
	c.logger.Debugf("[DriveWatcher] index updating in %v, currently containing %d nodes", time.Since(indexStart), len(c.index))
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
	// Cleaning the index
	// TODO
	// Done
	c.logger.Infof("[DriveWatcher] %d valid change(s) compiled in %v", len(changedFiles), time.Since(start))
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
		changesReq.Fields(googleapi.Field("nextPageToken"), googleapi.Field("newStartPageToken"), googleapi.Field("changes"),
			googleapi.Field("changes/fileId"), googleapi.Field("changes/removed"), googleapi.Field("changes/time"), googleapi.Field("changes/changeType"), googleapi.Field("changes/file"),
			googleapi.Field("changes/file/name"), googleapi.Field("changes/file/mimeType"), googleapi.Field("changes/file/trashed"), googleapi.Field("changes/file/parents"), googleapi.Field("changes/file/createdTime"))
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
		c.logger.Debug("[DriveWatcher] another page of changes is available")
		var nextPagesChanges []*drive.Change
		if nextPagesChanges, err = c.fetchChanges(changeList.NextPageToken); err != nil {
			err = fmt.Errorf("failed to get change list next page: %w", err)
			return
		}
		changes = append(changes, nextPagesChanges...)
		return
	}
	// We are the last page of results
	if changeList.NewStartPageToken != "" {
		c.logger.Debugf("[DriveWatcher] no more changes pages, recovering the marker for next run: %s", changeList.NewStartPageToken)
		// save new start token for next run
		c.startPageToken = changeList.NewStartPageToken
	} else {
		err = errors.New("end of changelist should contain NewStartPageToken")
	}
	// Done
	if c.logger.IsDebugShown() {
		for index, change := range changes {
			c.logger.Debugf("[DriveWatcher] raw change #%d: %+v", index+1, *change)
		}
	}
	return
}

func (c *Controller) incorporateChanges(changes []*drive.Change) (err error) {
	c.logger.Debugf("[DriveWatcher] start building the index based on %d change(s)", len(changes))
	// Build the file index starting by infos contained in the change list
	for _, change := range changes {
		// Skip is the change is drive metadata related
		if change.ChangeType != "file" {
			continue
		}
		// If file deleted, we won't have any information. To make it valid, we must have it within our index
		if change.Removed {
			c.logger.Warningf("[DriveWatcher] file change for fileID %s is removal, we won't have any data about it anymore", change.FileId) // TODO remove with statefull index
			continue
		}
		// Sometimes the file field come back empty, no idea why
		if change.File == nil {
			c.logger.Warningf("[DriveWatcher] file change for fileID %s had its file metadata empty, adding it to the lookup list", change.FileId) // TODO must be related to removal, reevalute with statefull index
			c.index[change.FileId] = nil
			continue
		}
		// Extract known info for this file
		c.index[change.FileId] = &filesIndexInfos{
			Name:        change.File.Name,
			MimeType:    change.File.MimeType,
			Parents:     change.File.Parents,
			Trashed:     change.File.Trashed,
			CreatedTime: change.File.CreatedTime,
		}
		// Add its parents for search
		for _, parent := range change.File.Parents {
			c.index[parent] = nil
		}
	}
	// Found out all missing parents infos
	if err = c.consolidateIndex(); err != nil {
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
	var (
		fileName     string
		fileMimeType string
		fileTrashed  bool
		fileCreated  string
	)
	if change.File != nil {
		fileName = change.File.Name
		fileMimeType = change.File.MimeType
		fileTrashed = change.File.Trashed
		fileCreated = change.File.CreatedTime
	} else {
		var (
			fi    *filesIndexInfos
			found bool
		)
		if fi, found = c.index[change.FileId]; !found {
			if change.Removed {
				c.logger.Warningf("[DriveWatcher] fileID %s has been removed but it is not within our index: we can not compute its path and therefor will be skipped",
					change.FileId)
			} else {
				err = fmt.Errorf("change does not contain file metadata and its fileID '%s' was not found within the index", change.FileId)
			}
			return
		}
		fileName = fi.Name
		fileMimeType = fi.MimeType
		fileTrashed = fi.Trashed
		fileCreated = fi.CreatedTime
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
	createdTime, err := time.Parse(time.RFC3339, fileCreated)
	if err != nil {
		err = fmt.Errorf("failed to convert create time for fileID %s, name '%s': %w", change.FileId, fileName, err)
		return
	}
	// Save up the consolidated info for return collection
	fc = &fileChange{
		Event:   changeTime,
		Folder:  fileMimeType == folderMimeType,
		Deleted: change.Removed || fileTrashed,
		Created: createdTime,
		Paths:   validPaths,
	}
	return
}
