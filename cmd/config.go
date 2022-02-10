package main

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/rclone/rclone/vfs/vfscommon"
)

const (
	rcloneConfigPathEnvName        = "RCGDIP_RCLONE_CONFIG_PATH"
	rcloneDriveBackendNameEnvName  = "RCGDIP_RCLONE_BACKEND_DRIVE_NAME"
	rcloneDrivePollIntervalEnvName = "RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL"
	rcloneCryptackendNameEnvName   = "RCGDIP_RCLONE_BACKEND_CRYPT_NAME"
	rcloneMountPathEnvName         = "RCGDIP_RCLONE_MOUNT_PATH"
	plexURLEnvName                 = "RCGDIP_PLEX_URL"
	plexTokenEnvName               = "RCGDIP_PLEX_TOKEN"
)

var (
	rcloneConfigPath        string
	rcloneDriveName         string
	rcloneDrivePollInterval time.Duration
	rcloneCryptName         string
	rcloneMountPath         string
	plexURL                 *url.URL
	plexToken               string
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
			return fmt.Errorf("%s can not be set under a second", rcloneDrivePollIntervalEnvName)
		}
	} else {
		// use rclone default
		rcloneDrivePollInterval = vfscommon.DefaultOpt.PollInterval
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
	return
}
