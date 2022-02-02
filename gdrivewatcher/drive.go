package gdrivewatcher

import (
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	maxFilesPerPage = 1000
	folderMimeType  = "application/vnd.google-apps.folder"
)

func (c *Controller) initDriveClient() (err error) {
	// Prepare the OAuth2 configuration
	oauthConf := &oauth2.Config{
		Scopes:       []string{scopePrefix + c.rc.Drive.Scope},
		Endpoint:     google.Endpoint,
		ClientID:     c.rc.Drive.ClientID,
		ClientSecret: c.rc.Drive.ClientSecret,
		// RedirectURL:  oauthutil.TitleBarRedirectURL,
	}
	// Init the HTTP OAuth2 enabled client
	client := oauthConf.Client(c.ctx, c.rc.Drive.Token)
	// Init Drive API client on top of that
	c.driveClient, err = drive.NewService(c.ctx, option.WithHTTPClient(client))
	return
}

func (c *Controller) validateRemoteDrive() (valid bool) {
	// we won't use mutex here as this fx is only called during init
	c.logger.Info("[DriveWatcher] validating state...")
	// If the remote drive does not validate, invalid our local state
	defer func() {
		if !valid {
			c.state = stateData{}
		}
	}()
	// First do we have one ?
	if c.state.RootID == "" {
		c.logger.Info("[DriveWatcher] no root folderID found, starting a new state")
		return
	}
	// Get the stored rootID to see if we are still accessing the same drive
	fileInfos, err := c.getFileInfo(c.state.RootID)
	if err != nil {
		c.logger.Warningf("[DriveWatcher] can not get our cached root fileID infos from remote, invalidating state: %w", err)
		return
	}
	// Check
	if len(fileInfos.Parents) != 0 {
		c.logger.Warningf("[DriveWatcher] our cached root fileID has parents, invalidating state: %w", err)
		return
	}
	// All good
	c.logger.Debugf("[DriveWatcher] the root folderID '%s' in our local state seems valid", c.state.RootID)
	return true
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

type driveFileBasicInfo struct {
	Name    string
	Folder  bool
	Parents []string
}

func (c *Controller) getFileInfo(fileID string) (infos *driveFileBasicInfo, err error) {
	c.logger.Debugf("[DriveWatcher] requesting information about fileID %s...", fileID)
	// Build request
	fileRequest := c.driveClient.Files.Get(fileID).Context(c.ctx)
	fileRequest.Fields(googleapi.Field("name"), googleapi.Field("mimeType"), googleapi.Field("parents"), googleapi.Field("trashed"), googleapi.Field("createdTime"))
	if c.rc.Drive.TeamDrive != "" {
		fileRequest.SupportsAllDrives(true)
	}
	// Execute request
	if err = c.limiter.Wait(c.ctx); err != nil {
		err = fmt.Errorf("can not execute API request, waiting for the limiter failed: %w", err)
		return
	}
	start := time.Now()
	fii, err := fileRequest.Do()
	if err != nil {
		err = fmt.Errorf("failed to execute file info get API query: %w", err)
		return
	}
	c.logger.Debugf("[DriveWatcher] information about fileID %s recovered in %v", fileID, time.Since(start))
	// Extract data
	infos = &driveFileBasicInfo{
		Name:    fii.Name,
		Folder:  fii.MimeType == folderMimeType,
		Parents: fii.Parents,
	}
	return
}
