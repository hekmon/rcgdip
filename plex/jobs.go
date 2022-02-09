package plex

import (
	"strings"
	"time"

	plexapi "github.com/hekmon/rcgdip/plex/api"
)

const (
	waitTimeSafetyMargin = time.Second
)

type jobElement struct {
	LibKey   string
	LibName  string
	ScanAt   time.Time
	ScanPath string
}

func (c *Controller) generateJobsDefinition(path string, eventTime time.Time, libs []plexapi.Library) (jobs []jobElement) {
	// Find libraries that contains this path
	validLibs := make(map[string]string, len(libs))
libs:
	for _, lib := range libs {
		for _, location := range lib.Locations {
			if strings.HasPrefix(path, location) {
				validLibs[lib.Key] = lib.Title
				c.logger.Debugf("[Plex] library '%s' has a location containing '%s' which needs (re)scan: adding to job creation list",
					lib.Title, path)
				continue libs
			}
		}
	}
	if len(validLibs) == 0 {
		return
	}
	// Compute the time when we will be able to start the scan (+ a safety marging)
	waitUntil := eventTime.Add(c.interval + waitTimeSafetyMargin)
	// Create the jobs definition
	jobs = make([]jobElement, len(validLibs))
	index := 0
	for libKey, libName := range validLibs {
		jobs[index] = jobElement{
			LibKey:   libKey,
			LibName:  libName,
			ScanAt:   waitUntil,
			ScanPath: path,
		}
		index++
	}
	return
}

func (c *Controller) jobExecutor(job jobElement) {
	// A job executor is a goroutine started by the worker
	defer c.workers.Done()
	// Compute how long we should wait for the poll interval of the rclone mount point to be matched for sure
	timer := time.NewTimer(time.Until(job.ScanAt))
	select {
	case <-timer.C:
		// Time's up let's scan this lib/path
		if _, err := c.plex.ScanLibrary(c.ctx, job.LibKey, job.ScanPath); err != nil {
			c.logger.Errorf("[Plex] failed to start partial library scan for '%s' on path '%s': %s", job.LibName, job.ScanPath, err)
		} else {
			c.logger.Infof("[Plex] successfully launched a partial scan for '%s' on path '%s'", job.LibName, job.ScanPath)
		}
	case <-c.ctx.Done():
		// we should stop, let's save the job in our state
		timer.Stop()
		c.logger.Infof("[Plex] scan job for '%s' on '%s' is not yet launched, saving into state...", job.LibName, job.ScanPath)
		// TODO
	}
}
