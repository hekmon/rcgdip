package gdrivewatcher

import (
	"errors"
	"fmt"
	"time"
)

type driveFileBasicInfo struct {
	Name    string   `json:"name"`
	Folder  bool     `json:"isFolder"`
	Parents []string `json:"parentsID"`
}

func (c *Controller) initialIndexBuild() (rootFolderID string, err error) {
	c.logger.Infof("[DriveWatcher] building the initial index...")
	// Get all the things, ahem files
	start := time.Now()
	files, err := c.getListPage("")
	if err != nil {
		err = fmt.Errorf("recovering file listgin from Google Drive failed: %w", err)
		return
	}
	// Build the index with the infos
	lookupList := make([]string, 0, len(files))
	for _, file := range files {
		// Add file info to the index
		if err = c.index.Set(file.Id, driveFileBasicInfo{
			Name:    file.Name,
			Folder:  file.MimeType == folderMimeType,
			Parents: file.Parents,
		}); err != nil {
			err = fmt.Errorf("failed to save file infos for fileID '%s' within the local index: %w", file.Id, err)
			return
		}
		// Mark its parents for search during consolidate (actually all parents are within the listing except... the root folder)
		for _, parent := range file.Parents {
			if !c.index.Has(parent) {
				lookupList = append(lookupList, parent)
			}
		}
	}
	// Consolidate (for absolute root folder id)
	if err = c.recursivelyDiscoverFiles(lookupList); err != nil {
		err = fmt.Errorf("failed to consolidate index after initial build up: %w", err)
		return
	}
	// Check we have a root folder ID within our index after populating it
	if rootFolderID, err = c.getIndexRootFolder(); err != nil {
		err = fmt.Errorf("failed to recover the root folder ID within our index: %w", err)
		return
	}
	if rootFolderID == "" {
		err = errors.New("something must have gone wrong during the index building: can not find the root folder fileID")
		return
	}
	// Done
	if c.logger.IsInfoShown() {
		// c.index.NbKeys() is filtered so a bit expensive
		c.logger.Infof("[DriveWatcher] index builded with %d nodes in %v", c.index.NbKeys(), time.Since(start))
	}
	return
}

func (c *Controller) recursivelyDiscoverFiles(ids []string) (err error) {
	var (
		fileInfo *driveFileBasicInfo
	)
	// Search all IDs
	lookupList := make([]string, 0, len(ids))
	for _, fileID := range ids {
		// Get file infos
		if fileInfo, err = c.getFileInfo(fileID); err != nil {
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

func (c *Controller) getIndexRootFolder() (rootFolderID string, err error) {
	var fileInfos driveFileBasicInfo
	for _, fileID := range c.index.Keys() {
		if _, err = c.index.Get(fileID, &fileInfos); err != nil {
			err = fmt.Errorf("failed to get info from index for fileID '%s': %w", fileID, err)
			return
		}
		if len(fileInfos.Parents) == 0 {
			rootFolderID = fileID
			return
		}
	}
	return
}
