package plex

import (
	"strings"
	"time"

	plexapi "github.com/hekmon/rcgdip/plex/api"
)

type jobElement struct {
	LibKey string
	ScanAt time.Time
	Path   string
}

func (c *Controller) generateJobsDefinition(path string, eventTime time.Time, libs []plexapi.Library) (jobs []jobElement) {
	// Find libraries that contains this path
	validLibs := make([]string, 0, len(libs))
libs:
	for _, lib := range libs {
		for _, location := range lib.Locations {
			if strings.HasPrefix(path, location) {
				validLibs = append(validLibs, lib.Key)
				c.logger.Debugf("[Plex] library '%s' has a location containing '%s' which needs (re)scan: adding to job creation list",
					lib.Title, path)
				continue libs
			}
		}
	}
	if len(validLibs) == 0 {
		return
	}
	// Compute the time when we will be able to start the scan
	waitUntil := eventTime.Add(c.interval)
	// Create the jobs definition
	jobs = make([]jobElement, len(validLibs))
	for index, libKey := range validLibs {
		jobs[index] = jobElement{
			LibKey: libKey,
			ScanAt: waitUntil,
			Path:   path,
		}
	}
	return
}
