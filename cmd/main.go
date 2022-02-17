package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hekmon/rcgdip/drivechange"
	"github.com/hekmon/rcgdip/gdrive"
	"github.com/hekmon/rcgdip/gdrive/rcsnooper"
	"github.com/hekmon/rcgdip/plex"
	"github.com/hekmon/rcgdip/storage"

	"github.com/hekmon/hllogger/v2"
	sysd "github.com/iguanesolutions/go-systemd/v5"
	sysdnotify "github.com/iguanesolutions/go-systemd/v5/notify"
)

var (
	// Linking time
	appName    = "rcgdip"
	appVersion = "dev"
	// Flags
	systemdLaunched bool
	// Controllers
	logger        *hllogger.Logger
	db            *storage.Controller
	driveWatcher  *gdrive.Controller
	plexTriggerer *plex.Controller
	// Clean stop
	mainCtx       context.Context
	mainCtxCancel func()
	mainStop      chan struct{}
)

func main() {
	// Process flags
	flagVersion := flag.Bool("version", false, "show version")
	flagInstance := flag.String("instance", "", "define a custom instance for storage")
	flag.Parse()
	if *flagVersion {
		fmt.Printf("%s %s\n", appName, appVersion)
		os.Exit(0)
	}

	// Get config
	err := populateConfig()
	if err != nil {
		log.Fatal(1, fmt.Sprintf("[Main] invalid configuration: %s", err))
	}

	// Probe execution environment
	_, systemdLaunched = sysd.GetInvocationID()

	// Initialize the logger
	logger = hllogger.New(os.Stdout, logLevel)
	if systemdLaunched {
		logger.Debug("[Main] systemd integration activated")
	}

	// Debug output of the conf
	if logger.IsDebugShown() {
		debugConf()
	}

	// Prepare clean stop
	mainCtx, mainCtxCancel = context.WithCancel(context.Background())
	mainStop = make(chan struct{})
	go handleSignals()
	go stopper()

	// Init storage
	logger.Info("[Main] initializing the storage backend...")
	if db, err = storage.New(storage.Config{
		Instance: *flagInstance,
		Logger:   logger,
	}); err != nil {
		logger.Errorf("[Main] failed to initialize storage: %s", err.Error())
		os.Exit(1)
	}
	logger.Info("[Main] storage backend ready")

	// Prepare the communication channel
	changesChan := make(chan []drivechange.File)

	// Initialize GDrive controller
	logger.Info("[Main] initializing the Google Drive watcher...")
	if driveWatcher, err = gdrive.New(mainCtx, gdrive.Config{
		RClone: rcsnooper.Config{
			RCloneConfigPath: rcloneConfigPath,
			DriveBackendName: rcloneDriveName,
			CryptBackendName: rcloneCryptName,
		},
		PollInterval: rcloneDrivePollInterval,
		Logger:       logger,
		StateBackend: db.NewScoppedAccess("drive_state"),
		IndexBackend: db.NewScoppedAccess("drive_index"),
		KillSwitch:   killSwtich,
		Output:       changesChan,
	}); err != nil {
		logger.Errorf("[Main] failed to initialize the Google Drive watcher: %s", err.Error())
		mainCtxCancel()
		<-mainStop
		os.Exit(2)
	}
	logger.Info("[Main] Google Drive watcher started")

	// Initialize the Plex controller
	logger.Info("[Main] initializing the Plex Triggerer...")
	if plexTriggerer, err = plex.New(mainCtx, plex.Config{
		Input:          changesChan,
		PollInterval:   rcloneDrivePollInterval,
		DirCacheTime:   rcloneDriveDirCacheTime,
		MountPoint:     rcloneMountPath,
		PlexURL:        plexURL,
		PlexToken:      plexToken,
		ProductName:    appName,
		ProductVersion: appVersion,
		StateBackend:   db.NewScoppedAccess("plex_state"),
		Logger:         logger,
	}); err != nil {
		logger.Errorf("[Main] failed to initialize the Plex Triggerer: %s", err.Error())
		mainCtxCancel()
		<-mainStop
		os.Exit(3)
	}
	logger.Info("[Main] Plex Triggerer started")

	// We are ready
	if err = sysdnotify.Ready(); err != nil {
		logger.Errorf("[Main] can't send systemd ready notification: %v", err)
	}
	<-mainStop
	logger.Debugf("[Main] clean stop ok, exiting")
	os.Exit(exitCode)
}
