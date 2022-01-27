package gdrivewatcher

import (
	"errors"
	"fmt"
	"path"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

const (
	gdriveFields      = "name,parents"
	maxChangesPerPage = 1000
	folderMimeType    = "application/vnd.google-apps.folder"
)

func (c *Controller) Changes() (err error) {
	// Dev: fake init
	changesStart, err := c.driveClient.Changes.GetStartPageToken().Context(c.ctx).Do()
	if err != nil {
		return
	}
	c.startPageToken = changesStart.StartPageToken
	fmt.Println("Waiting", 30*time.Second)
	time.Sleep(30 * time.Second)
	// Save the start token in case something goes wrong for future retry
	backupStartToken := c.startPageToken
	defer func() {
		if err != nil {
			c.startPageToken = backupStartToken
		}
	}()
	// Get changes
	changes, err := c.getChanges(c.startPageToken)
	if err != nil {
		err = fmt.Errorf("failed to get all changes recursively: %w", err)
		return
	}
	// Enrich infos about the ones we need
	filesIndex, err := c.expandChanges(changes)
	if err != nil {
		err = fmt.Errorf("failed to process the %d changes retreived: %w", len(changes), err)
		return
	}
	// Compute the paths containing changes
	changesPaths, err := c.getChangesPath(changes, filesIndex)
	if err != nil {
		err = fmt.Errorf("failed to compute the changes paths from the %d changes retreived: %w", len(changes), err)
		return
	}
	fmt.Println(changesPaths)
	return
}

/*
	Layer 1 - get all changes
*/

func (c *Controller) getChanges(nextPageToken string) (changes []*drive.Change, err error) {
	fmt.Println("change request")
	// Build Request
	changesReq := c.driveClient.Changes.List(nextPageToken).Context(c.ctx)
	// changesReq.PageSize(1)
	changesReq.PageSize(maxChangesPerPage)
	// changesReq.Fields(googleapi.Field("kind"), googleapi.Field("nextPageToken"), googleapi.Field("newStartPageToken"),
	// googleapi.Field("changes"), googleapi.Field("changes/fileId"))
	changesReq.Fields(googleapi.Field("*"))
	// Execute Request
	changeList, err := changesReq.Do()
	if err != nil {
		err = fmt.Errorf("failed to execute the API query for changes list: %w", err)
		return
	}
	// Extract changes
	changes = changeList.Changes
	// Are we the last page ?
	if changeList.NextPageToken == "" {
		if changeList.NewStartPageToken == "" {
			err = errors.New("end of changelist should contain NewStartPageToken")
		} else {
			// save new start token for next run
			c.startPageToken = changeList.NewStartPageToken
		}
		return
	}
	// If not, handle next pages recursively
	var nextPagesChanges []*drive.Change
	if nextPagesChanges, err = c.getChanges(changeList.NextPageToken); err != nil {
		err = fmt.Errorf("failed to get change list next page: %w", err)
		return
	}
	changes = append(changes, nextPagesChanges...)
	return
}

/*
	Layer 2 - Expand/Enrich
*/

func (c *Controller) expandChanges(changes []*drive.Change) (filesIndex filesInfo, err error) {
	filesIndex = make(filesInfo, len(changes))
	// Build the file index starting by infos contained in the change list
	for _, change := range changes {
		// Skip is the change is drive metadata related
		if change.ChangeType != "file" {
			continue
		}
		// Extract known info for this file
		filesIndex[change.FileId] = &fileInfo{
			Name:    change.File.Name,
			Folder:  change.File.MimeType == folderMimeType,
			Parents: change.File.Parents,
		}
		// Mark its parent for search
		for _, parent := range change.File.Parents {
			filesIndex[parent] = nil // mark for search
		}
	}
	// Found out all missing parents infos
	if err = c.getFilesParentsInfo(filesIndex); err != nil {
		err = fmt.Errorf("failed to recover all parents files infos: %w", err)
		return
	}
	for fileID, fileInfos := range filesIndex {
		fmt.Printf("%s: %+v\n", fileID, *fileInfos)
	}
	return
}

type filesInfo map[string]*fileInfo

func (c *Controller) getFilesParentsInfo(files filesInfo) (err error) {
	var runWithSearch, found bool
	// Check all fileIDs
	for fileID, fileInfos := range files {
		// Is this fileIDs already searched ?
		if fileInfos != nil {
			continue
		}
		// Get file infos
		if fileInfos, err = c.getFilePathInfo(fileID); err != nil {
			err = fmt.Errorf("failed to get file info for fileID %s: %w", fileID, err)
			return
		}
		// Save them
		files[fileID] = fileInfos
		// Prepare its parents for search if unknown
		for _, parent := range fileInfos.Parents {
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
	// No search done this run, time to return for real
	return
}

type fileInfo struct {
	Name    string
	Folder  bool
	Parents []string
}

func (c *Controller) getFilePathInfo(fileID string) (infos *fileInfo, err error) {
	// Build request
	fileRequest := c.driveClient.Files.Get(fileID).Context(c.ctx)
	fileRequest.Fields(googleapi.Field("name"), googleapi.Field("mimeType"), googleapi.Field("parents"))
	// Execute request
	fileInfos, err := fileRequest.Do()
	if err != nil {
		err = fmt.Errorf("failed to execute file info get API query: %w", err)
		return
	}
	// Extract data
	infos = &fileInfo{
		Name:    fileInfos.Name,
		Folder:  fileInfos.MimeType == folderMimeType,
		Parents: fileInfos.Parents,
	}
	return
}

/*
	Layer 3 - Get changes paths
*/

func (c *Controller) getChangesPath(changes []*drive.Change, filesIndex filesInfo) (changesPaths []string, err error) {
	rootID := getRootID(filesIndex)
	if rootID == "" {
		err = errors.New("impossible de find the root fileID, parent search must has been wrong")
		return
	}
	// Let's compute paths
	paths := make(map[string]struct{}, len(changes))
	var path string
	for _, change := range changes {
		// Skip is the change is drive metadata related
		if change.ChangeType != "file" {
			continue
		}
		// Compute paths
		if change.File.MimeType == folderMimeType {
			if path, err = generatePath(change.FileId, filesIndex); err != nil {
				err = fmt.Errorf("failed to generate path for fileID %s, name '%s': %w", change.FileId, change.File.Name, err)
				return
			}
			paths[path] = struct{}{}
		} else if hasParent(change.File.Parents, rootID) {
			paths["/"] = struct{}{}
		}
	}
	// Create the final slice of uniq paths
	changesPaths = make([]string, len(paths))
	i := 0
	for path := range paths {
		changesPaths[i] = path
		i++
	}
	return
}

func getRootID(files filesInfo) (rootID string) {
	for fileID, fileInfos := range files {
		if len(fileInfos.Parents) == 0 {
			rootID = fileID
			return
		}
	}
	return
}

func hasParent(parents []string, search string) (contains bool) {
	for _, parent := range parents {
		if parent == search {
			return true
		}
	}
	return
}

func generatePath(fileID string, filesIndex filesInfo) (buildedPath string, err error) {
	// Build the reverse stack
	reverseElems := make([]string, 0, len(filesIndex))
	for {
		// Obtain infos for current fileID
		fileInfos, found := filesIndex[fileID]
		if !found {
			err = fmt.Errorf("fileID '%s' not found", fileID)
			return
		}
		reverseElems = append(reverseElems, fileInfos.Name)
		// Stop if no parent
		if len(fileInfos.Parents) == 0 {
			break
		}
		// Else pick first and keep building up
		fileID = fileInfos.Parents[0]
	}
	// Inverse the stack
	elems := make([]string, len(reverseElems))
	for index, elem := range reverseElems {
		elems[len(reverseElems)-1-index] = elem
	}
	// Build and return the path
	buildedPath = path.Join(elems...)
	return
}
