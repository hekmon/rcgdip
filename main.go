package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/hekmon/rcgdip/gdrivewatcher"
	"github.com/hekmon/rcgdip/gdrivewatcher/rcsnooper"
	"github.com/hekmon/rcgdip/storage"

	"github.com/hekmon/hllogger"
	sysd "github.com/iguanesolutions/go-systemd/v5"
	sysdnotify "github.com/iguanesolutions/go-systemd/v5/notify"
)

var (
	// Flags
	systemdLaunched bool
	// Controllers
	logger       *hllogger.HlLogger
	db           *storage.Controller
	driveWatcher *gdrivewatcher.Controller
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

	// Initialize GDrive controller
	logger.Info("[Main] initializing the Google Drive watcher...")
	if driveWatcher, err = gdrivewatcher.New(mainCtx, gdrivewatcher.Config{
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
	}); err != nil {
		logger.Fatalf(1, "[Main] failed to initialize the Google Drive watcher: %s", err.Error())
	}
	logger.Info("[Main] Google Drive watcher started")

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
	wg.Add(1)
	go func() {
		driveWatcher.WaitUntilFullStop()
		wg.Done()
	}()
	// Wait for all
	wg.Wait()
	// All workers have exited, clean stop the db
	db.Stop()
}
