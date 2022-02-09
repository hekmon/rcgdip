package plex

import (
	"fmt"
	"strings"
	"time"

	plexapi "github.com/hekmon/rcgdip/plex/api"
)

const (
	waitTimeSafetyMargin = time.Second
	stateJobsTotalKey    = "jobs_len"
	stateJobsPrefix      = "jobs_#"
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
	// Create the jobs definitions
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
		c.logger.Infof("[Plex] scan job for '%s' on '%s' is not yet launched, saving for resume later...", job.LibName, job.ScanPath)
		c.jobsAccess.Lock()
		c.jobs = append(c.jobs, job)
		c.jobsAccess.Unlock()
	}
}

func (c *Controller) restoreJobs() {
	var (
		err               error
		found             bool
		totalJobsSaved    int
		totalJobsRestored int
		jobKey            string
		restoredJob       jobElement
	)
	// Get number of saved jobs
	if found, err = c.state.Get(stateJobsTotalKey, &totalJobsSaved); err != nil {
		c.logger.Errorf("[Plex] failed to load the total number of saved job(s), the db might have become inconsistent: %s",
			err)
		return
	}
	if !found {
		c.logger.Debugf("[Plex] saved jobs index not found in db, assuming no job need resuming")
		return
	}
	c.jobs = make([]jobElement, 0, totalJobsSaved)
	// Restore each job
	defer c.jobsAccess.Unlock()
	c.jobsAccess.Lock()
	for i := 0; i < totalJobsSaved; i++ {
		jobKey = jobsGenerateKey(i)
		// Get the job
		if found, err = c.state.Get(jobKey, &restoredJob); err != nil {
			c.logger.Errorf("[Plex] failed to restore the job #%d: %s", i, err)
			continue
		}
		if !found {
			c.logger.Errorf("[Plex] failed to restore the job #%d: not found within db (is db inconsistent ?)", i)
			continue
		}
		// Add it to the list of restored jobs
		c.jobs = append(c.jobs, restoredJob)
		totalJobsRestored++
		// Remove it from the db
		if err = c.state.Delete(jobKey); err != nil {
			c.logger.Errorf("[Plex] failed to delete within the db the restored job #%d, the db might have become inconsistent: %s", i, err)
		}
	}
	// Clean the number of saved jobs from db
	if err = c.state.Delete(stateJobsTotalKey); err != nil {
		c.logger.Errorf("[Plex] failed to delete total number of saved jobs within the db, it might have become inconsistent: %s", err)
	}
	// Done
	if totalJobsRestored > 0 {
		c.logger.Infof("[Plex] restored %d previously planned scan job(s)", totalJobsRestored)
	} else {
		c.logger.Debug("[Plex] no previously planned scan job found/restored")
	}
}

func (c *Controller) saveJobs() {
	var (
		err          error
		totalWritten int
	)
	// Save all jobs
	c.jobsAccess.Lock()
	for index, job := range c.jobs {
		if err = c.state.Set(jobsGenerateKey(totalWritten), job); err != nil {
			c.logger.Errorf("[Plex] failed to save the unstarted job #%d, job will be lost: %s @ %s", index, job.LibName, job.ScanPath)
		} else {
			totalWritten++
		}
	}
	c.jobsAccess.Unlock()
	// Save total
	if err = c.state.Set(stateJobsTotalKey, totalWritten); err != nil {
		c.logger.Errorf("[Plex] failed to save the total number of saved job(s), the db might have become inconsistent: %s",
			err)
	}
}

func jobsGenerateKey(jobIndex int) string {
	return fmt.Sprintf("%s%d", stateJobsPrefix, jobIndex)
}
