package gdrive

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hekmon/rcgdip/drivechange"
	"github.com/hekmon/rcgdip/gdrive/rcsnooper"

	"github.com/hekmon/hllogger/v2"
	"golang.org/x/time/rate"
	"google.golang.org/api/drive/v3"
)

type Config struct {
	RClone       rcsnooper.Config
	PollInterval time.Duration
	Logger       *hllogger.Logger
	StateBackend Storage
	IndexBackend Storage
	KillSwitch   func()
	Output       chan<- []drivechange.File
}

type Storage interface {
	Clear() error
	Delete(string) error
	Get(string, interface{}) (bool, error)
	Has(string) bool
	Keys() []string
	NbKeys() int
	Set(string, interface{}) error
	Sync() error
}

type Controller struct {
	// Global
	ctx        context.Context
	logger     *hllogger.Logger
	killSwitch func()
	// RClone Snooper
	rc *rcsnooper.Controller
	// Google Drive API client
	driveClient *drive.Service
	limiter     *rate.Limiter
	// Storage
	state Storage
	index Storage
	// Watcher info
	output chan<- []drivechange.File
	// Workers control plane
	workers  sync.WaitGroup
	fullStop chan struct{}
}

func New(ctx context.Context, conf Config) (c *Controller, err error) {
	defer func() {
		if err != nil {
			c = nil
		}
	}()
	// First we initialize the RClone config snooper
	rc, err := rcsnooper.New(conf.RClone)
	if err != nil {
		err = fmt.Errorf("failed to initialize the RClone controller: %w", err)
		return
	}
	conf.Logger.Infof("[Drive] %s", rc.Summary())
	// Then we initialize ourself
	c = &Controller{
		ctx:        ctx,
		logger:     conf.Logger,
		killSwitch: conf.KillSwitch,
		rc:         rc,
		limiter:    rate.NewLimiter(rate.Every(time.Minute/requestPerMin), requestPerMin/2),
		state:      conf.StateBackend,
		index:      conf.IndexBackend,
		output:     conf.Output,
	}
	if err = c.initDriveClient(); err != nil {
		err = fmt.Errorf("unable to initialize Drive API client: %w", err)
		return
	}
	// Some weird case
	if c.rc.Drive.Options.RootFolderID == "root" {
		c.rc.Drive.Options.RootFolderID = ""
	}
	// Workers
	c.fullStop = make(chan struct{})
	go c.stopper()
	c.workers.Add(1)
	go c.watcher(conf.PollInterval)
	// Done
	return
}

func (c *Controller) stopper() {
	// Waiting for stop signal
	<-c.ctx.Done()
	// Wait for workers to correctly stop
	c.logger.Debug("[Drive] waiting for all workers to stop...")
	c.workers.Wait()
	// Mark full stop
	close(c.fullStop)
	c.logger.Info("[Drive] fully stopped")
}

func (c *Controller) WaitUntilFullStop() {
	<-c.fullStop
}
