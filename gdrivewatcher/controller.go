package gdrivewatcher

import (
	"context"
	"fmt"
	"time"

	"github.com/hekmon/rcgdip/gdrivewatcher/rcsnooper"

	"github.com/hekmon/hllogger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	scopePrefix = "https://www.googleapis.com/auth/"
)

type Config struct {
	RClone rcsnooper.Config
	Logger *hllogger.HlLogger
}

type Controller struct {
	// Global
	ctx    context.Context
	logger *hllogger.HlLogger
	// RClone Snooper
	rc *rcsnooper.Controller
	// Google Drive API client
	driveClient    *drive.Service
	startPageToken string
	// Index related
	index filesIndex
}

func New(ctx context.Context, conf Config) (c *Controller, err error) {
	// First we initialize the RClone config snooper
	rc, err := rcsnooper.New(conf.RClone)
	if err != nil {
		err = fmt.Errorf("failed to initialize the RClone controller: %w", err)
		return
	}
	// Then we initialize ourself
	c = &Controller{
		ctx:    ctx,
		logger: conf.Logger,
		rc:     rc,
	}
	// Prepare the OAuth2 configuration
	oauthConf := &oauth2.Config{
		Scopes:       []string{scopePrefix + rc.Drive.Scope},
		Endpoint:     google.Endpoint,
		ClientID:     rc.Drive.ClientID,
		ClientSecret: rc.Drive.ClientSecret,
		// RedirectURL:  oauthutil.TitleBarRedirectURL,
	}
	client := oauthConf.Client(ctx, rc.Drive.Token)
	// Init Drive client
	if c.driveClient, err = drive.NewService(ctx, option.WithHTTPClient(client)); err != nil {
		err = fmt.Errorf("unable to initialize Drive API client: %w", err)
		return
	}
	// Done
	conf.Logger.Infof("[DriveWatcher] %s", rc.Summary())
	return
}

func (c *Controller) FakeRun() (err error) {
	// Dev: fake init
	changesReq := c.driveClient.Changes.GetStartPageToken().Context(c.ctx)
	if c.rc.Drive.TeamDrive != "" {
		changesReq.SupportsAllDrives(true).DriveId(c.rc.Drive.TeamDrive)
	}
	changesStart, err := changesReq.Do()
	if err != nil {
		return
	}
	c.startPageToken = changesStart.StartPageToken

	// Build the index
	if err = c.buildIndex(); err != nil {
		err = fmt.Errorf("failed to build the initial index: %w", err)
		return
	}

	// Do stuff
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
			decryptedPath, err := c.rc.CryptCipher.DecryptFileName(path)
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
