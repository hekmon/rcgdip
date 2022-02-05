package gdrivewatcher

import (
	"fmt"
	"time"
)

func (c *Controller) watcher(interval time.Duration) {
	// Prepare
	defer c.workers.Done()
	// Has the rclone backend changed ?
	if err := c.validateStateAgainstRemoteDrive(); err != nil {
		c.logger.Errorf("[DriveWatcher] failed to validate local state: %s", err)
		if c.ctx.Err() == nil {
			c.killSwitch()
		}
		return
	}
	// Fresh start ? (or reset)
	if err := c.populate(); err != nil {
		c.logger.Errorf("[DriveWatcher] failed to initialize local state: %s", err)
		if c.ctx.Err() == nil {
			c.killSwitch()
		}
		return
	}
	// Start the watch
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	c.logger.Infof("[DriveWatcher] will check for changes every %v", interval)
	for {
		select {
		case <-ticker.C:
			c.workerPass()
		case <-c.ctx.Done():
			c.logger.Debug("[DriveWatcher] stopping worker as main context has been cancelled")
			return
		}
	}
}

func (c *Controller) workerPass() {
	c.logger.Debug("[DriveWatcher] checking changes...")
	// Compute the paths containing changes
	changesFiles, err := c.getFilesChanges()
	if err != nil {
		c.logger.Errorf("failed to retreived changed files: %w", err)
		return
	}
	fmt.Println("---- CHANGED FILES ----")
	for _, change := range changesFiles {
		fmt.Printf("%v %v %v", change.Event, change.Deleted, change.Folder)
		for _, path := range change.Paths {
			fmt.Printf("\t%s", path)
			if c.rc.CryptCipher != nil {
				decryptedPath, err := c.rc.CryptCipher.DecryptFileName(path)
				if err != nil {
					panic(err)
				}
				fmt.Printf(" -> %s", decryptedPath)
			}
		}
		fmt.Println()
	}
	fmt.Println("--------")
}
