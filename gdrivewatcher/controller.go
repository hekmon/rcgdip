package gdrivewatcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hekmon/rcgdip/gdrivewatcher/rcsnooper"

	"github.com/hekmon/hllogger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/time/rate"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	scopePrefix   = "https://www.googleapis.com/auth/"
	requestPerMin = 300 // https://developers.google.com/docs/api/limits
)

type Config struct {
	RClone       rcsnooper.Config
	PollInterval time.Duration
	Logger       *hllogger.HlLogger
}

type Controller struct {
	// Global
	ctx    context.Context
	logger *hllogger.HlLogger
	// RClone Snooper
	rc *rcsnooper.Controller
	// Google Drive API client
	driveClient *drive.Service
	limiter     *rate.Limiter
	// State related
	index          filesIndex
	indexAccess    sync.RWMutex
	startPageToken string
	// Workers control plane
	workers  sync.WaitGroup
	fullStop chan struct{}
}

func New(ctx context.Context, conf Config) (c *Controller, err error) {
	// First we initialize the RClone config snooper
	rc, err := rcsnooper.New(conf.RClone)
	if err != nil {
		err = fmt.Errorf("failed to initialize the RClone controller: %w", err)
		return
	}
	conf.Logger.Infof("[DriveWatcher] %s", rc.Summary())
	// Then we initialize ourself
	c = &Controller{
		ctx:     ctx,
		logger:  conf.Logger,
		rc:      rc,
		limiter: rate.NewLimiter(rate.Every(time.Minute/requestPerMin), requestPerMin/3),
	}
	if err = c.initDriveClient(); err != nil {
		err = fmt.Errorf("unable to initialize Drive API client: %w", err)
		return
	}
	// Load state
	if err = c.restoreState(); err != nil {
		err = fmt.Errorf("failed to restore state: %w", err)
		return
	}
	// Has the rclone backend changed ?
	// TODO reset
	// Fresh start ?
	if c.startPageToken == "" {
		if err = c.getChangesStartPage(); err != nil {
			err = fmt.Errorf("failed to get the start page token from Drive API: %w", err)
			return
		}
	}
	if c.index == nil {
		if err = c.buildIndex(); err != nil {
			err = fmt.Errorf("failed to index the drive: %w", err)
			return
		}
	}
	// Workers
	c.fullStop = make(chan struct{})
	go c.stopper()
	c.workers.Add(1)
	go c.watcher(conf.PollInterval)
	// Done
	return
}

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

func (c *Controller) stopper() {
	var err error
	// Waiting for stop signal
	<-c.ctx.Done()
	// Wait for workers to correctly stop
	c.logger.Debug("[DriveWatcher] waiting for all workers to stop...")
	c.workers.Wait()
	// Save the stop
	c.logger.Debug("[DriveWatcher] all workers stopped")
	if err = c.SaveState(); err != nil {
		c.logger.Errorf("[DriveWatcher] failed to save the state: %s", err.Error())
	} else {
		c.logger.Infof("[DriveWatcher] state successfully saved into %s", stateFileName)
	}
	// Mark full stop
	close(c.fullStop)
}

func (c *Controller) WaitUntilFullStop() {
	<-c.fullStop
}

// TODO index mutex
// TODO nextpage mutex
