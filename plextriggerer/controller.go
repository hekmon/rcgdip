package plextriggerer

import (
	"context"
	"sync"
	"time"

	"github.com/hekmon/rcgdip/drivechange"

	"github.com/hekmon/hllogger"
)

type Config struct {
	Input        <-chan []drivechange.File
	PollInterval time.Duration
	Logger       *hllogger.HlLogger
}

type Controller struct {
	// Global
	ctx      context.Context
	interval time.Duration
	logger   *hllogger.HlLogger
	// Workers control plane
	workers  sync.WaitGroup
	fullStop chan struct{}
}

func New(ctx context.Context, conf Config) (c *Controller, err error) {
	// Base init
	c = &Controller{
		ctx:      ctx,
		interval: conf.PollInterval,
		logger:   conf.Logger,
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
}

func (c *Controller) WaitUntilFullStop() {
	<-c.fullStop
}
