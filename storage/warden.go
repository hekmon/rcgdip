package storage

import (
	"time"

	"github.com/hekmon/cunits/v2"
)

const (
	wardenFreq   = 1 * time.Minute
	minToReclain = cunits.Bits(10) * cunits.MiB
)

func (c *Controller) warden() {
	defer c.workers.Done()
	// Start the watch
	ticker := time.NewTicker(wardenFreq)
	defer ticker.Stop()
	c.logger.Debugf("[Storage] will check db every %v", wardenFreq)
	for {
		select {
		case <-ticker.C:
			c.wardenPass()
		case <-c.ctx.Done():
			c.logger.Debug("[Storage] stopping warden worker as main context has been cancelled")
			return
		}
	}
}

func (c *Controller) wardenPass() {
	c.logger.Debug("[Storage] checking db...")
	// Compact db
	reclaimableSize := cunits.ImportInByte(float64(c.db.Reclaimable()))
	if reclaimableSize > 0 {
		if reclaimableSize >= minToReclain {
			c.logger.Infof("[Storage] reclaiming %s disk space...", reclaimableSize)
			if err := c.db.Merge(); err != nil {
				c.logger.Errorf("[Storage] failed to reclaim %s of disk space: %s",
					reclaimableSize, err.Error())
			} else {
				c.logger.Infof("[Storage] successfully reclaimed %s of disk space", reclaimableSize)
			}
		} else {
			c.logger.Debugf("[Storage] reclaimable space is too low to performe a merge: %s < %s", reclaimableSize, minToReclain)
		}
	}
	// Show stats
	if stats, err := c.db.Stats(); err != nil {
		c.logger.Errorf("[Storage] failed to get db stats: %s", err.Error())
	} else {
		c.logger.Infof("[Storage] db stats: %d data files, %d keys for %s on disk",
			stats.Datafiles, stats.Keys, cunits.ImportInByte(float64(stats.Size)))
	}
}
