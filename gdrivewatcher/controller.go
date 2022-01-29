package gdrivewatcher

import (
	"context"
	"fmt"
	"time"

	"github.com/hekmon/rcgdip/rcsnooper"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	scopePrefix = "https://www.googleapis.com/auth/"
)

type Config struct {
	Drive     rcsnooper.DriveBackend
	DecryptFx func(string) (string, error)
}

type Controller struct {
	ctx context.Context
	// Google Drive API client
	driveClient    *drive.Service
	startPageToken string
	// Drive Config
	folderRootID string
	teamDrive    string
	// Decrypt
	decrypt func(string) (string, error)
}

func New(ctx context.Context, conf Config) (c *Controller, err error) {
	c = &Controller{
		ctx:          ctx,
		folderRootID: conf.Drive.RootFolderID,
		teamDrive:    conf.Drive.TeamDrive,
		decrypt:      conf.DecryptFx,
	}
	// OAuth2 configuration
	oauthConf := &oauth2.Config{
		Scopes:       []string{scopePrefix + conf.Drive.Scope},
		Endpoint:     google.Endpoint,
		ClientID:     conf.Drive.ClientID,
		ClientSecret: conf.Drive.ClientSecret,
		// RedirectURL:  oauthutil.TitleBarRedirectURL,
	}
	client := oauthConf.Client(ctx, conf.Drive.Token)
	// Init Drive client
	if c.driveClient, err = drive.NewService(ctx, option.WithHTTPClient(client)); err != nil {
		err = fmt.Errorf("unable to initialize Drive client: %w", err)
		return
	}
	return
}

func (c *Controller) FakeRun() (err error) {
	// Dev: fake init
	changesReq := c.driveClient.Changes.GetStartPageToken().Context(c.ctx)
	if c.teamDrive != "" {
		changesReq.SupportsAllDrives(true).DriveId(c.teamDrive)
	}
	changesStart, err := changesReq.Do()
	if err != nil {
		return
	}
	c.startPageToken = changesStart.StartPageToken
	fmt.Println("Waiting", 30*time.Second)
	time.Sleep(30 * time.Second)

	// Compute the paths containing changes
	changesFiles, err := c.GetFilesChanges()
	if err != nil {
		err = fmt.Errorf("failed to retreived changed files: %w", err)
		return
	}
	fmt.Println("---- CHANGED FILES ----")
	for _, change := range changesFiles {
		fmt.Printf("%v %v %v", change.Event, change.Deleted, change.Created)
		for _, path := range change.Paths {
			fmt.Printf("\t%s -> ", path)
			decryptedPath, err := c.decrypt(path)
			if err != nil {
				panic(err)
			}
			fmt.Print(decryptedPath)
		}
		fmt.Println()
	}
	fmt.Println("--------")
	return
}
