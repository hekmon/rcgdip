package gdrivewatcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hekmon/rcgdip/gdrivewatcher/rcsnooper"

	"github.com/hekmon/hllogger"
	"golang.org/x/time/rate"
	"google.golang.org/api/drive/v3"
)

type Config struct {
	RClone       rcsnooper.Config
	PollInterval time.Duration
	Logger       *hllogger.HlLogger
	StateBackend Storage
	IndexBackend Storage
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
	ctx    context.Context
	logger *hllogger.HlLogger
	// RClone Snooper
	rc *rcsnooper.Controller
	// Google Drive API client
	driveClient *drive.Service
	limiter     *rate.Limiter
	// Storage
	state Storage
	index Storage
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
		limiter: rate.NewLimiter(rate.Every(time.Minute/requestPerMin), requestPerMin/2),
		state:   conf.StateBackend,
		index:   conf.IndexBackend,
	}
	if err = c.initDriveClient(); err != nil {
		err = fmt.Errorf("unable to initialize Drive API client: %w", err)
		return
	}
	// Has the rclone backend changed ?
	if err = c.validateStateAgainstRemoteDrive(); err != nil {
		err = fmt.Errorf("failed to validate local state: %w", err)
		return
	}
	// Fresh start ? (or reset)
	if err = c.populate(); err != nil {
		return
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
	c.logger.Debug("[DriveWatcher] waiting for all workers to stop...")
	c.workers.Wait()
	// Mark full stop
	close(c.fullStop)
}

func (c *Controller) WaitUntilFullStop() {
	<-c.fullStop
}
