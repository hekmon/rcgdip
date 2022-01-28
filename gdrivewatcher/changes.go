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
	fmt.Println("--------")
	fmt.Println("CHANGES")
	for _, change := range changes {
		fmt.Println(change.FileId, change.File.Name)
	}
	fmt.Println("--------")
	// Search parents and build the index
	filesIndex, err := c.buildIndex(changes)
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
	fmt.Println("--------")
	fmt.Println("PATHS")
	fmt.Println(changesPaths)
	fmt.Println("--------")
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

func (c *Controller) buildIndex(changes []*drive.Change) (filesIndex filesInfo, err error) {
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
	// Every files has been searched and have their info now, time to return for real
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
	Layer 3 - Get file paths from changes
*/

func (c *Controller) getChangesPath(changes []*drive.Change, filesIndex filesInfo) (changesPaths []string, err error) {
	// Let's compute paths
	changesPaths = make([]string, 0, len(changes))
	var paths []string
	for _, change := range changes {
		// Skip if the change is drive metadata related or not a file
		if change.ChangeType != "file" || change.File.MimeType != folderMimeType {
			continue
		}
		// Compute possible paths if files
		if paths, err = generatePaths(change.FileId, filesIndex); err != nil {
			err = fmt.Errorf("failed to generate path for fileID %s, name '%s': %w", change.FileId, change.File.Name, err)
			return
		}
		changesPaths = append(changesPaths, paths...)
	}
	return
}

func generatePaths(fileID string, filesIndex filesInfo) (buildedPaths []string, err error) {
	// Get all paths breaken down by elements in bottom up
	reversedPathsElems, err := generatePathsLookup(fileID, filesIndex)
	if err != nil {
		return
	}
	// For each path, compute the merged in order path
	buildedPaths = make([]string, len(reversedPathsElems))
	for reversedPathElemsIndex, reversedPathElems := range reversedPathsElems {
		// Inverse the stack
		orderedElems := make([]string, len(reversedPathElems))
		for index, elem := range reversedPathElems {
			orderedElems[len(reversedPathElems)-1-index] = elem
		}
		// Build the path
		buildedPaths[reversedPathElemsIndex] = "/" + path.Join(orderedElems...)
	}
	return
}

func generatePathsLookup(fileID string, filesIndex filesInfo) (buildedPaths [][]string, err error) {
	// Obtain infos for current fileID
	fileInfos, found := filesIndex[fileID]
	if !found {
		err = fmt.Errorf("fileID '%s' not found", fileID)
		return
	}
	// Stop if no parent, we have reached root folder
	if len(fileInfos.Parents) == 0 {
		return
	}
	// Follow the white rabbit
	buildedPaths = make([][]string, len(fileInfos.Parents))
	var (
		parentPaths [][]string
		currentPath []string
	)
	for parentIndex, parent := range fileInfos.Parents {
		// Get paths for this parent
		if parentPaths, err = generatePathsLookup(parent, filesIndex); err != nil {
			err = fmt.Errorf("failed to lookup parent path for folderID '%s': %w", parent, err)
			return
		}
		// If parent is root folder, just add ourself in this path
		if parentPaths == nil {
			buildedPaths[parentIndex] = []string{fileInfos.Name}
			continue
		}
		// Else add paths to final return while prefixing with current file/folder name
		for _, parentPath := range parentPaths {
			currentPath = make([]string, len(parentPath)+1)
			currentPath[0] = fileInfos.Name
			for parentPathElemIndex, parentPathElem := range parentPath {
				currentPath[parentPathElemIndex+1] = parentPathElem
			}
			buildedPaths[parentIndex] = currentPath
		}
	}
	// All parents paths explored
	return
}
