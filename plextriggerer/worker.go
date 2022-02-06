package plextriggerer

import (
	"path"
	"strings"

	"github.com/hekmon/rcgdip/drivechange"
)

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
	// Build uniq fully qualified folder paths to scan
	var nbPaths int
	for _, change := range changes {
		nbPaths += len(change.Paths)
	}
	paths := make(map[string]struct{}, nbPaths)
	nbPaths = 0
	for _, change := range changes {
		for _, changePath := range change.Paths {
			if change.Folder {
				if change.Deleted {
					// add parent
					parent := path.Clean(c.mountPoint + path.Dir(changePath))
					paths[parent] = struct{}{}
					c.logger.Debugf("[Plex] folder '%s' deleted, adding its parent to scan list: %s", changePath, parent)
				} else {
					c.logger.Debugf("[Plex] skipping folder change: %s", changePath)
				}
			} else {
				// add parent
				parent := path.Clean(c.mountPoint + path.Dir(changePath))
				paths[parent] = struct{}{}
				c.logger.Debugf("[Plex] file '%s' modified, adding its parent to scan list: %s", changePath, parent)
			}
		}
	}
	// Detect if some paths are included within parents scheduled for scan
	for candidatePath := range paths {
		for evaluatedPath := range paths {
			if candidatePath == evaluatedPath {
				// do not compare against self
				continue
			}
			if strings.HasPrefix(evaluatedPath, candidatePath) {
				c.logger.Debugf("[Plex] path '%s' remove from scan list: its parent '%s' is already scheduled for scan",
					evaluatedPath, candidatePath)
				delete(paths, evaluatedPath)
			}
		}
	}
	// Final list of paths
	scanList := make([]string, len(paths))
	index := 0
	for path := range paths {
		scanList[index] = path
		index++
	}
	c.logger.Infof("[Plex] scheduling scan for the following paths: %s", strings.Join(scanList, ", "))
}
