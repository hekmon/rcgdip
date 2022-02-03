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
	requestPerMin   = 300 / 2 // Let's share with rclone https://developers.google.com/docs/api/limits
	scopePrefix     = "https://www.googleapis.com/auth/"
	folderMimeType  = "application/vnd.google-apps.folder"
	maxFilesPerPage = 1000
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

func (c *Controller) getDriveListing(pageToken string) (files []*drive.File, nextPageToken string, err error) {
	c.logger.Debug("[DriveWatcher] getting a new page of files...")
	// Build Request
	listReq := c.driveClient.Files.List()
	listReq.Spaces("drive").Q("trashed=false")
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
	c.logger.Debugf("[DriveWatcher] %d file(s) obtained in this page in %v", len(filesList.Files), time.Since(start))
	// Extract files from answer
	files = filesList.Files
	nextPageToken = filesList.NextPageToken
	// Done
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
	c.logger.Debugf("[DriveWatcher] requesting information about fileID '%s'...", fileID)
	// Build request
	fileRequest := c.driveClient.Files.Get(fileID).Context(c.ctx)
	fileRequest.Fields(googleapi.Field("id"), googleapi.Field("name"), googleapi.Field("mimeType"), googleapi.Field("parents"))
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
	c.logger.Debugf("[DriveWatcher] information about fileID '%s' recovered in %v", fileID, time.Since(start))
	// Extract data
	recoveredID = fii.Id
	infos = &driveFileBasicInfo{
		Name:    fii.Name,
		Folder:  fii.MimeType == folderMimeType,
		Parents: fii.Parents,
	}
	return
}
