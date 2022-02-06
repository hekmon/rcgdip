package plextriggerer

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/hekmon/rcgdip/drivechange"

	"github.com/hekmon/hllogger"
)

type Config struct {
	Input        <-chan []drivechange.File
	PollInterval time.Duration
	MountPoint   string
	Logger       *hllogger.HlLogger
}

type Controller struct {
	// Global
	ctx        context.Context
	interval   time.Duration
	mountPoint string
	logger     *hllogger.HlLogger
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
