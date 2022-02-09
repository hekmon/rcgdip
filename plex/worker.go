package plex

import (
	"path"
	"strings"
	"time"

	"github.com/hekmon/rcgdip/drivechange"
)

func (c *Controller) triggerWorker(input <-chan []drivechange.File) {
	// Prepare
	defer c.workers.Done()
	// Testing the plex connection
	c.testPlexConnection()
	// Launch all restored jobs (if any)
	c.jobsAccess.Lock()
	c.workers.Add(len(c.jobs))
	for jobIndex, job := range c.jobs {
		c.logger.Debugf("[Plex] starting restored job #%d: %s, %s, %v", jobIndex+1, job.LibName, job.ScanPath, job.ScanAt)
		go c.jobExecutor(job)
	}
	c.jobs = nil
	c.jobsAccess.Unlock()
	// Wake up for work or stop
	c.logger.Debug("[Plex] waiting for input")
	for {
		select {
		case batch := <-input:
			c.workerPass(batch)
		case <-c.ctx.Done():
			c.logger.Debug("[Plex] stopping worker as main context has been cancelled")
			return
		}
	}
}

func (c *Controller) testPlexConnection() {
	// Get libs
	libs, _, err := c.plex.GetLibraries(c.ctx)
	if err != nil {
		c.logger.Errorf("[Plex] failed to query the current libraries: %s", err.Error())
		return
	}
	// Check libs locations
	var (
		nbPaths      int
		nbCandidates int
	)
	for _, lib := range libs {
		nbPaths += len(lib.Locations)
		for _, location := range lib.Locations {
			if strings.HasPrefix(location, c.mountPoint) {
				nbCandidates++
			}
		}
	}
	if nbPaths == 0 {
		c.logger.Warning("[Plex] no location found in any library: change events won't trigger any scan")
	} else if nbCandidates == 0 {
		c.logger.Warningf("[Plex] found %d libraries based on %d locations but none are based on rclone mount point '%s': change events won't trigger any scan",
			len(libs), nbPaths, c.mountPoint)
	} else {
		c.logger.Infof("[Plex] found %d libraries based on %d locations on which %d are based on rclone mountpoint '%s'",
			len(libs), nbPaths, nbCandidates, c.mountPoint)
	}
}

func (c *Controller) workerPass(changes []drivechange.File) {
	c.logger.Debugf("[Plex] received a batch of %d change(s)", len(changes))
	// Build uniq fully qualified folder paths to scan
	scanList := c.extractBasePathsToScan(changes)
	if c.logger.IsInfoShown() {
		paths := make([]string, len(scanList))
		index := 0
		for path := range scanList {
			paths[index] = path
			index++
		}
		c.logger.Infof("[Plex] the following %d path(s) need scanning: %s", len(paths), strings.Join(paths, ", "))
	}
	// Get plex libs
	libs, _, err := c.plex.GetLibraries(c.ctx)
	if err != nil {
		c.logger.Errorf("[Plex] failed to query the current libraries, aborting this batch: %s", err.Error())
		return
	}
	// Create scan jobs for each path if we can
	jobs := make([]jobElement, 0, len(scanList)*len(libs))
	for path, eventTime := range scanList {
		jobs = append(jobs, c.generateJobsDefinition(path, eventTime, libs)...)
	}
	c.logger.Debugf("[Plex] created %d scan jobs", len(jobs))
	// Start or schedule the jobs
	c.workers.Add(len(jobs))
	for jobIndex, job := range jobs {
		c.logger.Debugf("[Plex] starting job #%d: %s, %s, %v", jobIndex+1, job.LibName, job.ScanPath, job.ScanAt)
		go c.jobExecutor(job)
	}
}

func (c *Controller) extractBasePathsToScan(changes []drivechange.File) (scanList map[string]time.Time) {
	// Extract uniq parents to scan for file changes
	var (
		nbPaths                  int
		found                    bool
		alreadyScheduledPathTime time.Time
	)
	for _, change := range changes {
		nbPaths += len(change.Paths)
	}
	scanList = make(map[string]time.Time, nbPaths)
	nbPaths = 0
	for _, change := range changes {
		for _, changePath := range change.Paths {
			// Do not process folders not deleted
			if change.Folder && !change.Deleted {
				c.logger.Debugf("[Plex] skipping folder change not being deletion: %s", changePath)
				continue
			}
			// Schedule scan for parent folder
			parent := path.Clean(c.mountPoint + path.Dir(changePath))
			if alreadyScheduledPathTime, found = scanList[parent]; !found {
				scanList[parent] = change.Event
				// Debug log
				if c.logger.IsDebugShown() {
					if change.Folder { // and deleted ofc
						c.logger.Debugf("[Plex] folder '%s' deleted, adding its parent to scan list: %s", changePath, parent)
					} else {
						var action string
						if change.Deleted {
							action = "deleted"
						} else {
							action = "created or changed"
						}
						c.logger.Debugf("[Plex] file '%s' %s, adding its parent to scan list: %s", changePath, action, parent)
					}
				}
			} else if alreadyScheduledPathTime.Before(change.Event) {
				// current event is fresher thanthe one  previously registered for this path, this means we
				// need to wait longer to see it locally, always use the one we need to wait for the most to avoid scanning too early
				c.logger.Debugf("[Plex] path '%s' was already registered for scan for event at %v. But a new event is younger, replacing time: %v",
					parent, alreadyScheduledPathTime, change.Event)
				scanList[parent] = change.Event
			}
		}
	}
	// Detect if some paths are included within parents scheduled for scan
	for potentialParentPath, potentialParentTime := range scanList {
		for potentialChildPath, potentialChildTime := range scanList {
			if potentialParentPath == potentialChildPath {
				// do not compare against self
				continue
			}
			if strings.HasPrefix(potentialChildPath, potentialParentPath) {
				c.logger.Debugf("[Plex] path '%s' remove from scan list: its parent '%s' is already scheduled for scan",
					potentialChildPath, potentialParentPath)
				delete(scanList, potentialChildPath)
				// If child was to be scanned later than parent, delay the parent to allow both of them to appear on the mount
				if potentialChildTime.After(potentialParentTime) {
					c.logger.Debugf("[Plex] delaying the scan of the parent '%s' (event at %v) because the removed child path (%s) to be scan was scheduled later (event at %v)",
						potentialParentPath, potentialParentTime, potentialChildPath, potentialChildTime)
					scanList[potentialParentPath] = potentialChildTime
				}
			}
		}
	}
	return
}
