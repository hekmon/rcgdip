package storage

import (
	"time"

	"github.com/hekmon/cunits/v2"
)

const (
	wardenFreq          = 10 * time.Minute
	minPercentToReclain = 0.1
	minSizeToReclaim    = cunits.Bits(10) * cunits.MiB
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
	// Show stats
	stats, err := c.db.Stats()
	if err != nil {
		c.logger.Errorf("[Storage] failed to get db stats: %s", err.Error())
		return
	}
	totalSize := cunits.ImportInByte(float64(stats.Size))
	c.logger.Infof("[Storage] db stats: %d data files, %d keys for %s on disk",
		stats.Datafiles, stats.Keys, totalSize)
	// Compact db
	reclaimableSize := cunits.ImportInByte(float64(c.db.Reclaimable()))
	if reclaimableSize > 0 {
		percentReclaimable := float64(reclaimableSize) / float64(totalSize)
		if percentReclaimable >= minPercentToReclain || reclaimableSize >= minSizeToReclaim {
			c.logger.Infof("[Storage] reclaiming %s (%.02f%% of total db size) disk space...",
				reclaimableSize, percentReclaimable*100)
			if err := c.db.Merge(); err != nil {
				c.logger.Errorf("[Storage] failed to reclaim disk space: %s", err.Error())
			} else {
				c.logger.Infof("[Storage] successfully reclaimed %s (%.02f%% of total db size) of disk space",
					reclaimableSize, percentReclaimable*100)
			}
		} else {
			c.logger.Debugf("[Storage] reclaimable space is too low to performe a merge: %.02f%% representing %s",
				percentReclaimable*100, reclaimableSize)
		}
	}
}
