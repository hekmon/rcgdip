package plextriggerer

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/hekmon/rcgdip/drivechange"

	"github.com/hekmon/hllogger"
	"github.com/jrudio/go-plex-client"
)

type Config struct {
	Input        <-chan []drivechange.File
	PollInterval time.Duration
	MountPoint   string
	PlexURL      string
	PlexToken    string
	Logger       *hllogger.HlLogger
}

type Controller struct {
	// Global
	ctx        context.Context
	interval   time.Duration
	mountPoint string
	// Controllers
	logger *hllogger.HlLogger
	plex   *plex.Plex
	// Workers control plane
	workers  sync.WaitGroup
	fullStop chan struct{}
}

func New(ctx context.Context, conf Config) (c *Controller, err error) {
	// Base init
	c = &Controller{
		ctx:        ctx,
		interval:   conf.PollInterval,
		mountPoint: path.Clean(conf.MountPoint),
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
	// Init the plex client
	if c.plex, err = plex.New(conf.PlexURL, conf.PlexToken); err != nil {
		err = fmt.Errorf("failed to initialized plex client to '%s': %w", conf.PlexURL, err)
		return
	}
	plexOK, err := c.plex.Test()
	if err != nil {
		err = fmt.Errorf("failed to check plex connection to '%s': %w", conf.PlexURL, err)
		return
	}
	if !plexOK {
		err = fmt.Errorf("can not connect to plex at '%s'", conf.PlexURL)
		return
	}
	c.logger.Debugf("[Plex] successfully connected to remote plex server")
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
	// Mark full stop
	close(c.fullStop)
	c.logger.Debug("[Plex] fully stopped")
}

func (c *Controller) WaitUntilFullStop() {
	<-c.fullStop
}
