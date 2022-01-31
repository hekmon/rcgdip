package gdrivewatcher

import (
	"fmt"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

const (
	maxFilesPerPage = 1000
)

type filesIndex map[string]*filesIndexInfos

type filesIndexInfos struct {
	Name        string
	MimeType    string
	Parents     []string
	Trashed     bool
	CreatedTime string
}

func (c *Controller) buildIndex() (err error) {
	c.logger.Infof("[DriveWatcher] Building the initial index...")
	// Get all the things, ahem files
	start := time.Now()
	files, err := c.getListPage("")
	if err != nil {
		err = fmt.Errorf("recovering file listgin from Google Drive failed: %w", err)
		return
	}
	// Build the index with the infos
	c.index = make(filesIndex, len(files))
	for _, file := range files {
		c.index[file.Id] = &filesIndexInfos{
			Name:        file.Name,
			MimeType:    file.MimeType,
			Parents:     file.Parents,
			CreatedTime: file.CreatedTime,
		}
	}
	// Done
	c.logger.Infof("[DriveWatcher] index builded with %d nodes in %v", len(c.index), time.Since(start))
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
