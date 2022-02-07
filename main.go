package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/hekmon/rcgdip/drivechange"
	"github.com/hekmon/rcgdip/gdrive"
	"github.com/hekmon/rcgdip/gdrive/rcsnooper"
	"github.com/hekmon/rcgdip/plex"
	"github.com/hekmon/rcgdip/storage"

	"github.com/hekmon/hllogger"
	sysd "github.com/iguanesolutions/go-systemd/v5"
	sysdnotify "github.com/iguanesolutions/go-systemd/v5/notify"
)

var (
	// Linking time
	appName    = "rcgdip"
	appVersion = "0.0.0"
	// Flags
	systemdLaunched bool
	// Controllers
	logger        *hllogger.HlLogger
	db            *storage.Controller
	driveWatcher  *gdrive.Controller
	plexTriggerer *plex.Controller
	// Clean stop
	mainCtx       context.Context
	mainCtxCancel func()
	mainStop      chan struct{}
)

func main() {
	// Probe execution environment
	_, systemdLaunched = sysd.GetInvocationID()

	// Initialize the logger
	var loggerFlags int
	if !systemdLaunched {
		loggerFlags = hllogger.Ltime | hllogger.Ldate
	}
	logger = hllogger.New(os.Stdout, &hllogger.Config{
		LogLevel:              hllogger.Debug,
		LoggerFlags:           loggerFlags,
		SystemdJournaldCompat: systemdLaunched,
	})

	// Prepare clean stop
	mainCtx, mainCtxCancel = context.WithCancel(context.Background())
	mainStop = make(chan struct{})
	go handleSignals()
	go stopper()

	// Init storage
	var err error
	logger.Info("[Main] initializing the storage backend...")
	if db, err = storage.New(storage.Config{
		Instance: devinstance,
		Logger:   logger,
	}); err != nil {
		logger.Fatalf(1, "[Main] failed to initialize storage: %s", err.Error())
	}
	logger.Info("[Main] storage backend ready")

	// Prepare the communication channel
	changesChan := make(chan []drivechange.File)

	// Initialize GDrive controller
	logger.Info("[Main] initializing the Google Drive watcher...")
	if driveWatcher, err = gdrive.New(mainCtx, gdrive.Config{
		RClone: rcsnooper.Config{
			RCloneConfigPath: devrcloneconfigpath,
			DriveBackendName: devdrivebackendname,
			CryptBackendName: devcryptbackendname,
		},
		PollInterval: devpollinterval,
		Logger:       logger,
		StateBackend: db.NewScoppedAccess("drive_state"),
		IndexBackend: db.NewScoppedAccess("drive_index"),
		KillSwitch:   mainCtxCancel,
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
		PollInterval:   devpollinterval,
		MountPoint:     devmountpoint,
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
	if err := sysdnotify.Ready(); err != nil {
		logger.Errorf("[Main] can't send systemd ready notification: %v", err)
	}
	<-mainStop
	logger.Debugf("[Main] clean stop ok, exiting")
}

func handleSignals() {
	// Register signals
	var sig os.Signal
	signalChannel := make(chan os.Signal, 3)
	signal.Notify(signalChannel, syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1)
	// Waiting for signals to catch
	for sig = range signalChannel {
		switch sig {
		case syscall.SIGUSR1:
			// TODO backup db
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
