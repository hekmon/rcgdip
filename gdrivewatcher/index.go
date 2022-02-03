package gdrivewatcher

import (
	"fmt"
	"time"

	"google.golang.org/api/drive/v3"
)

type driveFileBasicInfo struct {
	Name    string   `json:"name"`
	Folder  bool     `json:"isFolder"`
	Parents []string `json:"parentsID"`
}

func (c *Controller) initialIndexBuild() (err error) {
	c.logger.Infof("[DriveWatcher] building the initial index...")
	start := time.Now()
	// Get all the things, ahem files
	var (
		pageFiles        []*drive.File
		nextPageToken    string
		lastStatsUpdate  time.Time
		pagesFetched     int
		nbFilesRecovered int
	)
	for {
		// Get page listing
		if pageFiles, nextPageToken, err = c.getDriveListing(nextPageToken); err != nil {
			err = fmt.Errorf("recovering file listing from Google Drive failed: %w", err)
			return
		}
		pagesFetched++
		nbFilesRecovered += len(pageFiles)
		// Build the index with the infos
		for _, file := range pageFiles {
			if err = c.index.Set(file.Id, driveFileBasicInfo{
				Name:    file.Name,
				Folder:  file.MimeType == folderMimeType,
				Parents: file.Parents,
			}); err != nil {
				err = fmt.Errorf("failed to save file infos for fileID '%s' within the local index: %w", file.Id, err)
				return
			}
		}
		// Listing over ?
		if nextPageToken == "" {
			break
		}
		// Put some stats out every minute as indexing can be quite long
		if time.Since(lastStatsUpdate) >= time.Minute {
			c.logger.Infof("[DriveWatcher] index building: so far %d list pages(s) has been recovered for a total of %d files",
				pagesFetched, nbFilesRecovered)
			lastStatsUpdate = time.Now()
		}
	}
	// Done
	if c.logger.IsInfoShown() {
		// c.index.NbKeys() is filtered so a bit expensive
		c.logger.Infof("[DriveWatcher] index builded with %d nodes in %v", c.index.NbKeys(), time.Since(start))
	}
	return
}

// TODO: check if not already in index before looking them up
func (c *Controller) recursivelyDiscoverFiles(ids []string) (err error) {
	var fileInfo *driveFileBasicInfo
	lookupList := make([]string, 0, len(ids))
	// Search all provided IDs
	for _, fileID := range ids {
		// Get file infos
		if fileInfo, err = c.getDriveFileInfo(fileID); err != nil {
			err = fmt.Errorf("failed to get file info for fileID %s: %w", fileID, err)
			return
		}
		// Save them
		if err = c.index.Set(fileID, driveFileBasicInfo{
			Name:    fileInfo.Name,
			Folder:  fileInfo.Folder,
			Parents: fileInfo.Parents,
		}); err != nil {
			err = fmt.Errorf("failed to save file infos for fileID '%s' within the local index: %w", fileID, err)
			return
		}
		// Prepare its parents for search if unknown
		for _, parent := range fileInfo.Parents {
			if !c.index.Has(parent) {
				lookupList = append(lookupList, parent)
			}
		}
	}
	if len(lookupList) > 0 {
		// new files infos discovered, let's find their parents too
		return c.recursivelyDiscoverFiles(lookupList)
	}
	// Every files has been searched and have their info now, time to return for real
	return
}
