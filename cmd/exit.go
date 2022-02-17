package main

import (
	"os"
	"sync"

	sysdnotify "github.com/iguanesolutions/go-systemd/v5/notify"
)

func killSwtich() {
	mainCtxCancel()
	<-mainStop
	os.Exit(4)
}

func stopper() {
	// If we exit, allow main goroutine to do so
	defer close(mainStop)
	// Wait for main context cancel
	<-mainCtx.Done()
	logger.Debugf("[Main] main context cancelled, stopping")
	// Systemd notify
	if err := sysdnotify.Stopping(); err != nil {
		logger.Errorf("[Main] can't send systemd stopping notification: %v", err)
	} else if systemdLaunched {
		logger.Debug("[Main] systemd stopping notification sent")
	}
	// Start workers waiters
	var wg sync.WaitGroup
	if driveWatcher != nil {
		wg.Add(1)
		go func() {
			driveWatcher.WaitUntilFullStop()
			wg.Done()
		}()
	}
	if plexTriggerer != nil {
		wg.Add(1)
		go func() {
			plexTriggerer.WaitUntilFullStop()
			wg.Done()
		}()
	}
	// Wait for all
	wg.Wait()
	// All workers have exited, clean stop the db
	db.Stop()
}
