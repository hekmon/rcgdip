package plex

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/hekmon/rcgdip/drivechange"
	plexapi "github.com/hekmon/rcgdip/plex/api"

	"github.com/hekmon/hllogger"
)

type Config struct {
	// Global config
	Input        <-chan []drivechange.File
	PollInterval time.Duration
	MountPoint   string
	// Plex API config
	PlexURL        string
	PlexToken      string
	ProductName    string
	ProductVersion string
	// Storage
	StateBackend Storage
	// Sub controllers
	Logger *hllogger.HlLogger
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
	interval   time.Duration
	mountPoint string
	tz         *time.Location
	// Storage
	state      Storage
	jobs       []*jobElement
	jobsAccess sync.Mutex
	// Controllers
	logger *hllogger.HlLogger
	plex   *plexapi.Client
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
	// Base init
	c = &Controller{
		ctx:        ctx,
		interval:   conf.PollInterval,
		mountPoint: path.Clean(conf.MountPoint),
		state:      conf.StateBackend,
		logger:     conf.Logger,
	}
	// Process mount point
	if !path.IsAbs(c.mountPoint) {
		err = fmt.Errorf("mount point path should be absolute: %s", c.mountPoint)
		return
	}
	if c.mountPoint[len(c.mountPoint)-1] != '/' {
		c.mountPoint += "/"
	}
	// Recover or generate a clientID
	clientID, err := c.getClientID()
	if err != nil {
		err = fmt.Errorf("failed to recover or generate a client ID for the plex API: %w", err)
		return
	}
	// Init the plex client
	if c.plex, err = plexapi.New(plexapi.Config{
		BaseURL:        conf.PlexURL,
		Token:          conf.PlexToken,
		ProductName:    conf.ProductName,
		ProductVersion: conf.ProductVersion,
		ClientID:       clientID,
	}); err != nil {
		err = fmt.Errorf("failed to instanciate the Plex API client: %w", err)
		return
	}
	// Get time location
	c.tz = time.Now().Location()
	// Restore jobs if needed
	c.restoreJobs()
	// Workers
	c.fullStop = make(chan struct{})
	go c.stopper()
	c.workers.Add(1)
	go c.triggerWorker(conf.Input)
	return
}

func (c *Controller) stopper() {
	// Waiting for stop signal
	<-c.ctx.Done()
	// Wait for workers to correctly stop
	c.logger.Debug("[Plex] waiting for all workers to stop...")
	c.workers.Wait()
	// Save unstarted jobs if any
	c.logger.Debug("[Plex] saving unstarted jobs to state...")
	c.saveJobs()
	// Mark full stop
	close(c.fullStop)
	c.logger.Info("[Plex] fully stopped")
}

func (c *Controller) WaitUntilFullStop() {
	<-c.fullStop
}
