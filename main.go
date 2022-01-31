package main

import (
	"context"
	"os"

	"github.com/hekmon/rcgdip/gdrivewatcher"
	"github.com/hekmon/rcgdip/gdrivewatcher/rcsnooper"

	"github.com/hekmon/hllogger"
	systemd "github.com/iguanesolutions/go-systemd/v5"
)

var (
	systemdLaunched bool
	logger          *hllogger.HlLogger
)

func main() {
	// Probe execution environment
	_, systemdLaunched = systemd.GetInvocationID()

	// Initialize the logger
	logger = hllogger.New(os.Stdout, &hllogger.Config{
		LogLevel:              hllogger.Debug,
		SystemdJournaldCompat: systemdLaunched,
	})

	// Initialize GDrive controller
	logger.Info("[Main] Initializing the Google Drive watcher...")
	gd, err := gdrivewatcher.New(context.TODO(), gdrivewatcher.Config{
		RClone: rcsnooper.Config{
			RCloneConfigPath: devrcloneconfigpath,
			DriveBackendName: devdrivebackendname,
			CryptBackendName: devcryptbackendname,
		},
		Logger: logger,
	})
	if err != nil {
		logger.Fatalf(1, "[Main] Failed to initialize the Google Drive watcher: %s", err.Error())
	}
	if err = gd.FakeRun(); err != nil {
		logger.Fatal(1, err.Error())
	}
}
