package gdrivewatcher

import (
	"errors"
	"fmt"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

const (
	maxFilesPerPage = 1000
)

type filesIndex map[string]*driveFileBasicInfo

func (c *Controller) buildIndex() (err error) {
	c.logger.Infof("[DriveWatcher] building the initial index...")
	// Get all the things, ahem files
	start := time.Now()
	files, err := c.getListPage("")
	if err != nil {
		err = fmt.Errorf("recovering file listgin from Google Drive failed: %w", err)
		return
	}
	// Build the index with the infos
	c.state.Index = make(filesIndex, len(files))
	for _, file := range files {
		// Add file info to the index
		c.state.Index[file.Id] = &driveFileBasicInfo{
			Name:        file.Name,
			MimeType:    file.MimeType,
			Parents:     file.Parents,
			CreatedTime: file.CreatedTime,
		}
		// Mark its parents for search during consolidate (actually all parents are within the listing except... the root folder)
		for _, parent := range file.Parents {
			if _, found := c.state.Index[parent]; !found {
				c.state.Index[parent] = nil
			}
		}
	}
	// Consolidate (for absolute root folder id)
	if err = c.consolidateIndex(); err != nil {
		err = fmt.Errorf("failed to consolidate index after initial build up: %w", err)
		return
	}
	// Check we have a root folder ID
	c.state.RootID = c.getIndexRootFolder()
	if c.state.RootID == "" {
		err = errors.New("something must have gone wrong during the index building: can not find the root folder fileID")
		return
	}
	// Done
	c.logger.Infof("[DriveWatcher] index builded with %d nodes in %v", len(c.state.Index), time.Since(start))
	return
}

func (c *Controller) getListPage(pageToken string) (files []*drive.File, err error) {
	c.logger.Debug("[DriveWatcher] getting a new page of files...")
	// Build Request
	listReq := c.driveClient.Files.List()
	listReq.Corpora("user").Spaces("drive")
	if c.rc.Drive.TeamDrive != "" {
		listReq.Corpora("drive").SupportsAllDrives(true).IncludeItemsFromAllDrives(true).DriveId(c.rc.Drive.TeamDrive)
	} else {
		listReq.Corpora("user")
	}
	if pageToken != "" {
		listReq.PageToken(pageToken)
	}
	{
		// // Dev
		// listReq.PageSize(1)
		// listReq.Fields(googleapi.Field("*"))
	}
	{
		// Prod
		listReq.PageSize(maxFilesPerPage)
		listReq.Fields(googleapi.Field("nextPageToken"), googleapi.Field("files/id"), googleapi.Field("files/name"),
			googleapi.Field("files/mimeType"), googleapi.Field("files/parents"), googleapi.Field("files/createdTime"))
	}
	// Execute Request
	if err = c.limiter.Wait(c.ctx); err != nil {
		err = fmt.Errorf("can not execute API request, waiting for the limiter failed: %w", err)
		return
	}
	start := time.Now()
	filesList, err := listReq.Do()
	if err != nil {
		err = fmt.Errorf("failed to execute the API query for files list: %w", err)
		return
	}
	c.logger.Debugf("[DriveWatcher] %d file(s) obtained in this page in %v", len(filesList.Files), time.Since(start))
	// Extract files from answer
	files = filesList.Files
	// Is there any pages left ?
	if filesList.NextPageToken != "" {
		c.logger.Debug("[DriveWatcher] another page of files is available")
		var nextPagesfiles []*drive.File
		if nextPagesfiles, err = c.getListPage(filesList.NextPageToken); err != nil {
			err = fmt.Errorf("failed to get change list next page: %w", err)
			return
		}
		files = append(files, nextPagesfiles...)
		return
	}
	// Done
	return
}

func (c *Controller) consolidateIndex() (err error) {
	var runWithSearch, found bool
	// Check all fileIDs
	for fileID, fileInfo := range c.state.Index {
		// Is this fileIDs already searched ?
		if fileInfo != nil {
			continue
		}
		// Get file infos
		if fileInfo, err = c.getFileInfo(fileID); err != nil {
			err = fmt.Errorf("failed to get file info for fileID %s: %w", fileID, err)
			return
		}
		// Save them
		c.state.Index[fileID] = fileInfo
		// Prepare its parents for search if unknown
		for _, parent := range fileInfo.Parents {
			if _, found = c.state.Index[parent]; !found {
				c.state.Index[parent] = nil
			}
		}
		// Mark this run as non empty
		runWithSearch = true
	}
	if runWithSearch {
		// new files infos discovered, let's find their parents too
		return c.consolidateIndex()
	}
	// Every files has been searched and have their info now, time to return for real
	return
}

func (c *Controller) getIndexRootFolder() string {
	for id, infos := range c.state.Index {
		if len(infos.Parents) == 0 {
			return id
		}
	}
	return ""
}
