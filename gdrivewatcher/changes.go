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
	changes, err := c.fetchChanges(c.startPageToken)
	if err != nil {
		err = fmt.Errorf("failed to get all changes recursively: %w", err)
		return
	}
	fmt.Println("---- CHANGES ----")
	for _, change := range changes {
		fmt.Printf("%+v\n%+v\n\n", *change, *change.File)
	}
	fmt.Println("--------")
	// Build the index with parents for further path computation
	index, err := c.buildIndex(changes)
	if err != nil {
		err = fmt.Errorf("failed to build up the parent index for the %d changes retreived: %w", len(changes), err)
		return
	}
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
	return
}

func (c *Controller) fetchChanges(nextPageToken string) (changes []*drive.Change, err error) {
	fmt.Println("change request")
	// Build Request
	changesReq := c.driveClient.Changes.List(nextPageToken).Context(c.ctx)
	changesReq.IncludeRemoved(true)
	{
		// Dev
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
	changeList, err := changesReq.Do()
	if err != nil {
		err = fmt.Errorf("failed to execute the API query for changes list: %w", err)
		return
	}
	// Extract changes from answer
	changes = changeList.Changes
	// Is there any pages left ?
	if changeList.NextPageToken != "" {
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
		// save new start token for next run
		c.startPageToken = changeList.NewStartPageToken
	} else {
		err = errors.New("end of changelist should contain NewStartPageToken")
	}
	return
}

type filesIndex map[string]*filesIndexInfos

type filesIndexInfos struct {
	Name    string
	Folder  bool
	Parents []string
}

func (c *Controller) buildIndex(changes []*drive.Change) (index filesIndex, err error) {
	index = make(filesIndex, len(changes))
	// Build the file index starting by infos contained in the change list
	for _, change := range changes {
		// Skip is the change is drive metadata related
		if change.ChangeType != "file" {
			continue
		}
		// Extract known info for this file
		index[change.FileId] = &filesIndexInfos{
			Name:    change.File.Name,
			Folder:  change.File.MimeType == folderMimeType,
			Parents: change.File.Parents,
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
	fmt.Println("---- INDEX ----")
	for fileID, filesIndexInfoss := range index {
		fmt.Printf("%s: %+v\n", fileID, *filesIndexInfoss)
	}
	fmt.Println("--------")
	return
}

func (c *Controller) getFilesParentsInfo(files filesIndex) (err error) {
	var runWithSearch, found bool
	// Check all fileIDs
	for fileID, filesIndexInfoss := range files {
		// Is this fileIDs already searched ?
		if filesIndexInfoss != nil {
			continue
		}
		// Get file infos
		if filesIndexInfoss, err = c.getfilesIndexInfos(fileID); err != nil {
			err = fmt.Errorf("failed to get file info for fileID %s: %w", fileID, err)
			return
		}
		// Save them
		files[fileID] = filesIndexInfoss
		// Prepare its parents for search if unknown
		for _, parent := range filesIndexInfoss.Parents {
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
	// Build request
	fileRequest := c.driveClient.Files.Get(fileID).Context(c.ctx)
	fileRequest.Fields(googleapi.Field("name"), googleapi.Field("mimeType"), googleapi.Field("parents"))
	// Execute request
	filesIndexInfoss, err := fileRequest.Do()
	if err != nil {
		err = fmt.Errorf("failed to execute file info get API query: %w", err)
		return
	}
	// Extract data
	infos = &filesIndexInfos{
		Name:    filesIndexInfoss.Name,
		Folder:  filesIndexInfoss.MimeType == folderMimeType,
		Parents: filesIndexInfoss.Parents,
	}
	return
}

func (c *Controller) processChange(change *drive.Change, index filesIndex) (fc *fileChange, err error) {
	// Skip if the change is drive metadata related or not a file
	if change.ChangeType != "file" || change.File.MimeType == folderMimeType {
		return
	}
	// Compute possible paths (bottom up)
	reversedPaths, err := generateReversePaths(change.FileId, index)
	if err != nil {
		err = fmt.Errorf("failed to generate path for fileID %s, name '%s': %w", change.FileId, change.File.Name, err)
		return
	}
	// Validate and reverse the paths (from bottom up to top down) to be exploitables
	validPaths := make([]string, 0, len(reversedPaths))
	for _, reversedPath := range reversedPaths {
		// If custom root folder id, search it and rewrite paths with new root
		if c.folderRootID != "" {
			if !reversedPath.CutAt(c.folderRootID) {
				fmt.Printf("path '%s' does not contain the custom root folder id, discarding it\n", reversedPath.Reverse().Path())
				continue // root folder id not found in this path, skipping
			}
		}
		// Path valid, adding it to the list
		validPaths = append(validPaths, reversedPath.Reverse().Path())
	}
	if len(validPaths) == 0 {
		// no valid path found (because of root folder id) skipping this change
		fmt.Printf("change for file '%s' does not contain any valid path, discarding it\n", change.File.Name)
		return
	}
	// Convert times
	changeTime, err := time.Parse(time.RFC3339, change.Time)
	if err != nil {
		err = fmt.Errorf("failed to convert change time for fileID %s, name '%s': %w", change.FileId, change.File.Name, err)
		return
	}
	createdTime, err := time.Parse(time.RFC3339, change.File.CreatedTime)
	if err != nil {
		err = fmt.Errorf("failed to convert create time for fileID %s, name '%s': %w", change.FileId, change.File.Name, err)
		return
	}
	// Save up the consolidated info for return collection
	fc = &fileChange{
		Event:   changeTime,
		Deleted: change.Removed || change.File.Trashed,
		Created: createdTime,
		Paths:   validPaths,
	}
	return
}
