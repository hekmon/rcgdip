package gdrive

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	requestPerMin     = 300 / 2 // Let's share with rclone https://developers.google.com/docs/api/limits
	scopePrefix       = "https://www.googleapis.com/auth/"
	folderMimeType    = "application/vnd.google-apps.folder"
	maxFilesPerPage   = 1000
	maxChangesPerPage = 1000
	devMode           = false
)

func (c *Controller) initDriveClient() (err error) {
	// Prepare the OAuth2 configuration
	oauthConf := &oauth2.Config{
		Scopes:       []string{scopePrefix + c.rc.Drive.Options.Scope},
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

func (c *Controller) getDriveChangesStartPage() (changesStartToken string, err error) {
	// Get start page token
	changesReq := c.driveClient.Changes.GetStartPageToken().Context(c.ctx)
	if c.rc.Drive.Options.TeamDriveID != "" {
		changesReq.SupportsAllDrives(true).DriveId(c.rc.Drive.Options.TeamDriveID)
	}
	changesStart, err := changesReq.Do()
	if err != nil {
		return
	}
	changesStartToken = changesStart.StartPageToken
	return
}

func (c *Controller) getDriveListing(pageToken string) (files []*drive.File, nextPageToken string, err error) {
	c.logger.Debug("[Drive] getting a new page of files...")
	// Build Request
	listReq := c.driveClient.Files.List()
	listReq.Spaces("drive").Q("trashed=false")
	if c.rc.Drive.Options.TeamDriveID != "" {
		listReq.Corpora("drive").SupportsAllDrives(true).IncludeItemsFromAllDrives(true).DriveId(c.rc.Drive.Options.TeamDriveID)
	} else {
		listReq.Corpora("user")
	}
	if pageToken != "" {
		listReq.PageToken(pageToken)
	}
	if devMode {
		listReq.PageSize(1)
		listReq.Fields(googleapi.Field("*"))
	} else {
		listReq.PageSize(maxFilesPerPage)
		listReq.Fields(googleapi.Field("nextPageToken"), googleapi.Field("files/id"), googleapi.Field("files/name"),
			googleapi.Field("files/mimeType"), googleapi.Field("files/parents"))
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
	c.logger.Debugf("[Drive] %d file(s) obtained in this page in %v", len(filesList.Files), time.Since(start))
	// Extract files from answer
	files = filesList.Files
	nextPageToken = filesList.NextPageToken
	// Done
	return
}

func (c *Controller) getDriveChanges(nextPageToken string) (changes []*drive.Change, nextStartPage string, err error) {
	c.logger.Debug("[Drive] getting a new page of changes...")
	// Build Request
	changesReq := c.driveClient.Changes.List(nextPageToken).Context(c.ctx)
	changesReq.IncludeRemoved(true)
	if c.rc.Drive.Options.TeamDriveID != "" {
		changesReq.SupportsAllDrives(true).IncludeItemsFromAllDrives(true).DriveId(c.rc.Drive.Options.TeamDriveID)
	}
	if devMode {
		changesReq.PageSize(1)
		changesReq.Fields(googleapi.Field("*"))
	} else {
		changesReq.PageSize(maxChangesPerPage)
		changesReq.Fields(googleapi.Field("nextPageToken"), googleapi.Field("newStartPageToken"),
			googleapi.Field("changes"), googleapi.Field("changes/fileId"), googleapi.Field("changes/removed"),
			googleapi.Field("changes/time"), googleapi.Field("changes/changeType"), googleapi.Field("changes/file"),
			googleapi.Field("changes/file/name"), googleapi.Field("changes/file/mimeType"), googleapi.Field("changes/file/trashed"),
			googleapi.Field("changes/file/parents"), googleapi.Field("changes/file/createdTime"))
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
	c.logger.Debugf("[Drive] changes page obtained in %v", time.Since(start))
	// Extract changes from answer
	changes = changeList.Changes
	// Is there any pages left ?
	if changeList.NextPageToken != "" {
		c.logger.Debugf("[Drive] another page of changes is available at %s", changeList.NextPageToken)
		var nextPagesChanges []*drive.Change
		if nextPagesChanges, nextStartPage, err = c.getDriveChanges(changeList.NextPageToken); err != nil {
			err = fmt.Errorf("failed to get change list next page: %w", err)
			return
		}
		changes = append(changes, nextPagesChanges...)
		return
	}
	// We are the last page of results, recover token for next run
	if changeList.NewStartPageToken == "" {
		err = errors.New("end of changelist should contain NewStartPageToken")
		return
	}
	c.logger.Debugf("[Drive] no more changes pages, recovering the marker for next run: %s", changeList.NewStartPageToken)
	nextStartPage = changeList.NewStartPageToken
	// Done
	if c.logger.IsDebugShown() {
		for index, change := range changes {
			c.logger.Debugf("[Drive] raw change #%d: %+v", index+1, *change)
		}
	}
	return
}

func (c *Controller) getDriveRootFileInfo() (rootID string, infos *driveFileBasicInfo, err error) {
	return c.getDriveFileInfoWithID("root")
}

func (c *Controller) getDriveFileInfo(fileID string) (infos *driveFileBasicInfo, err error) {
	_, infos, err = c.getDriveFileInfoWithID(fileID)
	return
}

func (c *Controller) getDriveFileInfoWithID(fileID string) (recoveredID string, infos *driveFileBasicInfo, err error) {
	c.logger.Debugf("[Drive] requesting information about fileID '%s'...", fileID)
	// Build request
	fileRequest := c.driveClient.Files.Get(fileID).Context(c.ctx)
	fileRequest.Fields(googleapi.Field("id"), googleapi.Field("name"), googleapi.Field("mimeType"), googleapi.Field("parents"))
	if c.rc.Drive.Options.TeamDriveID != "" {
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
	c.logger.Debugf("[Drive] information about fileID '%s' recovered in %v", fileID, time.Since(start))
	// Extract data
	recoveredID = fii.Id
	infos = &driveFileBasicInfo{
		Name:    fii.Name,
		Folder:  fii.MimeType == folderMimeType,
		Parents: fii.Parents,
	}
	return
}
