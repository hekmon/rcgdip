package plextriggerer

import "github.com/hekmon/rcgdip/drivechange"

func (c *Controller) triggerWorker(input <-chan []drivechange.File) {
	// Prepare
	defer c.workers.Done()
	// Wake up for work or stop
	c.logger.Debug("[Plex] waiting for input")
	for {
		select {
		case batch := <-input:
			c.workerPass(batch)
		case <-c.ctx.Done():
			c.logger.Debug("[Drive] stopping watcher as main context has been cancelled")
			return
		}
	}
}

func (c *Controller) workerPass(changes []drivechange.File) {
	c.logger.Debugf("[Plex] received a batch of %d change(s)", len(changes))
}
