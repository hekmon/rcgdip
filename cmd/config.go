package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hekmon/hllogger"
	"github.com/rclone/rclone/vfs/vfscommon"
)

const (
	rcloneConfigPathEnvName         = "RCGDIP_RCLONE_CONFIG_PATH"
	rcloneDriveBackendNameEnvName   = "RCGDIP_RCLONE_BACKEND_DRIVE_NAME"
	rcloneDrivePollIntervalEnvName  = "RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL"
	rcloneDriveDirCacheTimelEnvName = "RCGDIP_RCLONE_BACKEND_DRIVE_DIRCACHETIME"
	rcloneCryptackendNameEnvName    = "RCGDIP_RCLONE_BACKEND_CRYPT_NAME"
	rcloneMountPathEnvName          = "RCGDIP_RCLONE_MOUNT_PATH"
	plexURLEnvName                  = "RCGDIP_PLEX_URL"
	plexTokenEnvName                = "RCGDIP_PLEX_TOKEN"
	logLevelEnvName                 = "RCGDIP_LOGLEVEL"
)

var (
	rcloneConfigPath        string
	rcloneDriveName         string
	rcloneDrivePollInterval time.Duration
	rcloneDriveDirCacheTime time.Duration
	rcloneCryptName         string
	rcloneMountPath         string
	plexURL                 *url.URL
	plexToken               string
	logLevel                hllogger.LogLevel
)

func populateConfig() (err error) {
	// rclone config path
	if rcloneConfigPath = os.Getenv(rcloneConfigPathEnvName); rcloneConfigPath == "" {
		return fmt.Errorf("%s must be set", rcloneConfigPathEnvName)
	}
	if _, err = os.Stat(rcloneConfigPath); err != nil {
		return fmt.Errorf("can not access rclone config file at '%s': %s", rcloneConfigPath, err)
	}
	// backend names
	if rcloneDriveName = os.Getenv(rcloneDriveBackendNameEnvName); rcloneDriveName == "" {
		return fmt.Errorf("%s must be set", rcloneDriveBackendNameEnvName)
	}
	rcloneCryptName = os.Getenv(rcloneCryptackendNameEnvName)
	// poll interval
	pollIntervalStr := os.Getenv(rcloneDrivePollIntervalEnvName)
	if pollIntervalStr != "" {
		// parse
		if rcloneDrivePollInterval, err = time.ParseDuration(pollIntervalStr); err != nil {
			return fmt.Errorf("failed to parse %s as duration: %s", rcloneDrivePollIntervalEnvName, err)
		}
		if rcloneDrivePollInterval < time.Second {
			return fmt.Errorf("%s (%v) can not be set under a second", rcloneDrivePollIntervalEnvName, rcloneDrivePollInterval)
		}
	} else {
		// use rclone default
		rcloneDrivePollInterval = vfscommon.DefaultOpt.PollInterval
	}
	// dir cache time
	dirCacheTimeStr := os.Getenv(rcloneDriveDirCacheTimelEnvName)
	if pollIntervalStr != "" {
		// parse
		if rcloneDriveDirCacheTime, err = time.ParseDuration(dirCacheTimeStr); err != nil {
			return fmt.Errorf("failed to parse %s as duration: %s", rcloneDriveDirCacheTimelEnvName, err)
		}
		if rcloneDriveDirCacheTime < rcloneDrivePollInterval {
			return fmt.Errorf("%s (%v) can not be set lower than %s (%v)",
				rcloneDriveDirCacheTimelEnvName, rcloneDriveDirCacheTime, rcloneDrivePollIntervalEnvName, rcloneDrivePollInterval)
		}
	} else {
		// use rclone default
		rcloneDriveDirCacheTime = vfscommon.DefaultOpt.DirCacheTime
	}
	// mount path
	if rcloneMountPath = os.Getenv(rcloneMountPathEnvName); rcloneMountPath == "" {
		return fmt.Errorf("%s must be set", rcloneMountPath)
	}
	if rcloneMountPath[0] != '/' {
		return fmt.Errorf("%s must be absolute (it must start by '/')", rcloneMountPath)
	}
	// plex url
	plexURLStr := os.Getenv(plexURLEnvName)
	if plexURLStr == "" {
		return fmt.Errorf("%s must be set", plexURLEnvName)
	}
	if plexURL, err = url.Parse(plexURLStr); err != nil {
		return fmt.Errorf("failed to parse %s value as URL: %s", plexURLEnvName, err)
	}
	// plex token
	if plexToken = os.Getenv(plexTokenEnvName); plexToken == "" {
		return fmt.Errorf("%s must be set", plexTokenEnvName)
	}
	// log level
	switch strings.ToUpper(os.Getenv(logLevelEnvName)) {
	case "FATAL":
		logLevel = hllogger.Fatal
	case "ERROR":
		logLevel = hllogger.Error
	case "WARNING":
		logLevel = hllogger.Warning
	case "INFO":
		logLevel = hllogger.Info
	case "DEBUG":
		logLevel = hllogger.Debug
	default:
		logLevel = hllogger.Info
	}
	return
}

func debugConf() {
	logger.Debugf("[Main] %s: %s", rcloneConfigPathEnvName, rcloneConfigPath)
	logger.Debugf("[Main] %s: %s", rcloneDriveBackendNameEnvName, rcloneDriveName)
	logger.Debugf("[Main] %s: %v", rcloneDrivePollIntervalEnvName, rcloneDrivePollInterval)
	logger.Debugf("[Main] %s: %v", rcloneDriveDirCacheTimelEnvName, rcloneDriveDirCacheTime)
	logger.Debugf("[Main] %s: %v", rcloneCryptackendNameEnvName, rcloneCryptName)
	logger.Debugf("[Main] %s: %v", rcloneMountPathEnvName, rcloneMountPath)
	logger.Debugf("[Main] %s: %v", plexURLEnvName, plexURL.String())
	logger.Debugf("[Main] %s: <redacted>", plexTokenEnvName)
}
