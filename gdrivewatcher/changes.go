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
	index, err := c.buildIndex(changes)
	if err != nil {
		err = fmt.Errorf("failed to build up the parent index for the %d changes retreived: %w", len(changes), err)
		return
	}
	c.logger.Debugf("[DriveWatcher] index builded in %v, containing %d nodes", time.Since(indexStart), len(index))
	// Process each event
	changedFiles = make([]fileChange, 0, len(changes))
	var fc *fileChange
	for _, change := range changes {
		// Transforme change into a suitable file event
		if fc, err = c.processChange(change, index); err != nil {
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

type filesIndex map[string]*filesIndexInfos

type filesIndexInfos struct {
	Name        string
	MimeType    string
	Parents     []string
	Trashed     bool
	CreatedTime string
}

func (c *Controller) buildIndex(changes []*drive.Change) (index filesIndex, err error) {
	c.logger.Debugf("[DriveWatcher] start building the index based on %d change(s)", len(changes))
	index = make(filesIndex, len(changes))
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
			c.logger.Warningf("[DriveWatcher] file change for fileID %s had its file metadata empty, adding it to the lookup list", change.FileId)
			index[change.FileId] = nil
			continue
		}
		// Extract known info for this file
		index[change.FileId] = &filesIndexInfos{
			Name:        change.File.Name,
			MimeType:    change.File.MimeType,
			Parents:     change.File.Parents,
			Trashed:     change.File.Trashed,
			CreatedTime: change.File.CreatedTime,
		}
		// Add its parents for search
		for _, parent := range change.File.Parents {
			index[parent] = nil
		}
	}
	// Found out all missing parents infos
	if err = c.getFilesParentsInfo(index); err != nil {
		err = fmt.Errorf("failed to recover all parents files infos: %w", err)
		return
	}
	// Done
	if c.logger.IsDebugShown() {
		for fileID, filesIndexInfos := range index {
			c.logger.Debugf("[DriveWatcher] index fileID %s: %+v", fileID, *filesIndexInfos)
		}
	}
	return
}

func (c *Controller) getFilesParentsInfo(files filesIndex) (err error) {
	var runWithSearch, found bool
	// Check all fileIDs
	for fileID, fii := range files {
		// Is this fileIDs already searched ?
		if fii != nil {
			continue
		}
		// Get file infos
		if fii, err = c.getfilesIndexInfos(fileID); err != nil {
			err = fmt.Errorf("failed to get file info for fileID %s: %w", fileID, err)
			return
		}
		// Save them
		files[fileID] = fii
		// Prepare its parents for search if unknown
		for _, parent := range fii.Parents {
			if _, found = files[parent]; !found {
				files[parent] = nil
			}
		}
		// Mark this run as non empty
		runWithSearch = true
	}
	if runWithSearch {
		// new files infos discovered, let's find their parents too
		return c.getFilesParentsInfo(files)
	}
	// Every files has been searched and have their info now, time to return for real
	return
}

func (c *Controller) getfilesIndexInfos(fileID string) (infos *filesIndexInfos, err error) {
	c.logger.Debugf("[DriveWatcher] requesting information about fileID %s...", fileID)
	// Build request
	fileRequest := c.driveClient.Files.Get(fileID).Context(c.ctx)
	fileRequest.Fields(googleapi.Field("name"), googleapi.Field("mimeType"), googleapi.Field("parents"), googleapi.Field("trashed"), googleapi.Field("createdTime"))
	if c.rc.Drive.TeamDrive != "" {
		fileRequest.SupportsAllDrives(true)
	}
	// Execute request
	start := time.Now()
	fii, err := fileRequest.Do()
	if err != nil {
		err = fmt.Errorf("failed to execute file info get API query: %w", err)
		return
	}
	c.logger.Debugf("[DriveWatcher] information about fileID %s recovered in %v", fileID, time.Since(start))
	// Extract data
	infos = &filesIndexInfos{
		Name:        fii.Name,
		MimeType:    fii.MimeType,
		Parents:     fii.Parents,
		Trashed:     fii.Trashed,
		CreatedTime: fii.CreatedTime,
	}
	return
}

func (c *Controller) processChange(change *drive.Change, index filesIndex) (fc *fileChange, err error) {
	// Skip if the change is drive metadata related
	if change.ChangeType != "file" {
		return
	}
	// In case the file metadata was not provided within the change, extract info from our index
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
		if fi, found = index[change.FileId]; !found {
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
	reversedPaths, err := generateReversePaths(change.FileId, index)
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
