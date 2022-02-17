package main

import (
	"os"
	"os/signal"
	"syscall"

	sysdnotify "github.com/iguanesolutions/go-systemd/v5/notify"
)

func handleSignals() {
	// Register signals
	var sig os.Signal
	signalChannel := make(chan os.Signal, 3)
	signal.Notify(signalChannel, syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1)
	// Waiting for signals to catch
	for sig = range signalChannel {
		switch sig {
		case syscall.SIGUSR1:
			backupDB()
		case syscall.SIGTERM:
			fallthrough
		case syscall.SIGINT:
			logger.Output("\n")
			logger.Infof("[Main] signal '%v' caught: initiating clean stop", sig)
			mainCtxCancel()
			return
		default:
			logger.Warningf("[Main] Signal '%v' caught but no process set to handle it: skipping", sig)
		}
	}
}

func backupDB() {
	// Systemd notify
	if err := sysdnotify.Reloading(); err != nil {
		logger.Errorf("[Main] can't send systemd reloading notification: %v", err)
	} else if systemdLaunched {
		logger.Debug("[Main] systemd reloading notification sent")
	}
	// Backup db
	db.Backup()
	// Systemd notify
	if err := sysdnotify.Ready(); err != nil {
		logger.Errorf("[Main] can't send systemd ready notification: %v", err)
	} else if systemdLaunched {
		logger.Debug("[Main] systemd ready notification sent")
	}
}
