package gdrivewatcher

import (
	"time"
)

func (c *Controller) watcher(interval time.Duration) {
	// Prepare
	defer c.workers.Done()
	// Has the rclone backend changed ?
	var (
		sameDrive bool
		err       error
	)
	if sameDrive, err = c.validateStateAgainstRemoteDrive(); err != nil {
		c.logger.Errorf("[Drive] failed to validate if remote drive has changed: %s", err)
		if c.ctx.Err() == nil {
			c.killSwitch()
		}
		return
	}
	// Fresh start ? (or reset)
	if err := c.initState(!sameDrive); err != nil {
		c.logger.Errorf("[Drive] failed to initialize local state: %s", err)
		if c.ctx.Err() == nil {
			c.killSwitch()
		}
		return
	}
	// Start the watch
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	c.logger.Infof("[Drive] will check for changes every %v", interval)
	for {
		select {
		case <-ticker.C:
			c.workerPass()
		case <-c.ctx.Done():
			c.logger.Debug("[Drive] stopping watcher as main context has been cancelled")
			return
		}
	}
}

func (c *Controller) workerPass() {
	c.logger.Debug("[Drive] checking changes...")
	// Compute the paths containing changes
	changesFiles, err := c.getFilesChanges()
	if err != nil {
		c.logger.Errorf("failed to retreived changed files: %s", err)
		return
	}
	if len(changesFiles) == 0 {
		return
	}
	// Walk thru results to log and decrypt if needed
	var deletedSuffix string
	for changeIndex, change := range changesFiles {
		for pathIndex, path := range change.Paths {
			// Handle encryption if needed
			if c.rc.CryptCipher != nil {
				decryptedPath, err := c.rc.CryptCipher.DecryptFileName(path)
				if err != nil {
					c.logger.Errorf("[Drive] can not decrypt path '%s': %s", path, err)
					continue
				}
				c.logger.Debugf("[Drive] path decrypted using '%s' rclone backend configuration: %s  -->  %s",
					c.rc.Conf.CryptBackendName, path, decryptedPath)
				path = decryptedPath
				change.Paths[pathIndex] = path
			}
			// Print the changed files
			if !change.Folder {
				if change.Deleted {
					deletedSuffix = " (removed)"
				} else {
					deletedSuffix = ""
				}
				c.logger.Infof("[Drive] file change detected: %s%s", path, deletedSuffix)
			}
		}
		// Save the change with decrypted paths if needed
		if c.rc.CryptCipher != nil {
			changesFiles[changeIndex] = change
		}
	}
	// Send the collection to the consumer
	c.logger.Debug("[Drive] sending change(s)...")
	c.output <- changesFiles
	c.logger.Debugf("[Drive] sent %d change(s)", len(changesFiles))

}
