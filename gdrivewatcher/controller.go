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

func New(ctx context.Context, driveConfig rcsnooper.DriveBackend) (c *Controller, err error) {
	c = &Controller{
		ctx:          ctx,
		folderRootID: driveConfig.RootFolderID,
	}
	// OAuth2 configuration
	oauthConf := &oauth2.Config{
		Scopes:       []string{scopePrefix + driveConfig.Scope},
		Endpoint:     google.Endpoint,
		ClientID:     driveConfig.ClientID,
		ClientSecret: driveConfig.ClientSecret,
		// RedirectURL:  oauthutil.TitleBarRedirectURL,
	}
	client := oauthConf.Client(ctx, driveConfig.Token)
	// Init Drive client
	if c.driveClient, err = drive.NewService(ctx, option.WithHTTPClient(client)); err != nil {
		err = fmt.Errorf("unable to initialize Drive client: %w", err)
		return
	}
	return
}

type Controller struct {
	ctx context.Context
	// Google Drive API client
	driveClient    *drive.Service
	startPageToken string
	// Config
	folderRootID string
}

func (c *Controller) FakeRun() (err error) {
	// Dev: fake init
	changesStart, err := c.driveClient.Changes.GetStartPageToken().Context(c.ctx).Do()
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
	fmt.Printf("%+v\n", changesFiles)
	fmt.Println("--------")
	return
}
