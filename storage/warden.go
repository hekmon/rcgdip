package storage

import (
	"time"

	"github.com/hekmon/cunits/v2"
)

const (
	wardenFreq = 1 * time.Minute
)

func (c *Controller) warden() {
	defer c.workers.Done()
	// Start the watch
	ticker := time.NewTicker(wardenFreq)
	defer ticker.Stop()
	c.logger.Infof("[Storage] warden: will check db every %v", wardenFreq)
	for {
		select {
		case <-ticker.C:
			c.wardenPass()
		case <-c.ctx.Done():
			c.logger.Debug("[Storage] warden: stopping as main context has been cancelled")
			return
		}
	}
}

func (c *Controller) wardenPass() {
	c.logger.Debug("[Storage] warden: starting pass")
	// Compact db
	if reclaimableSize := c.db.Reclaimable(); reclaimableSize > 0 {
		size := cunits.ImportInByte(float64(reclaimableSize))
		c.logger.Debugf("[Storage] warden: reclaiming %d disk space...", size)
		if err := c.db.Merge(); err != nil {
			c.logger.Errorf("[Storaqe] warden: failed to reclaim %d of disk space: %s",
				size, err.Error())
		} else {
			c.logger.Infof("[Storaqe] warden: successfully reclaimed %d of disk space", size)
		}
	} else {
		c.logger.Debug("[Storage] warden: no reclaimable space found")
	}
	// Show stats
	if stats, err := c.db.Stats(); err != nil {
		c.logger.Errorf("[Storaqe] warden: failed to get db stats: %s", err.Error())
	} else {
		c.logger.Infof("[Storaqe] warden: db has %d data files, %d keys and weight %s",
			stats.Datafiles, stats.Keys, cunits.ImportInByte(float64(stats.Size)))
	}
}
