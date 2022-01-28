package rcsnooper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Unknwon/goconfig"
)

const (
	rcloneConfigTypeKey               = "type"
	rcloneConfigGDriveTypeValue       = "drive"
	rcloneConfigGDriveClientIDKey     = "client_id"
	rcloneConfigGDriveClientSecretKey = "client_secret"
	rcloneConfigGDriveScopeKey        = "scope"
	rcloneConfigGDriveTokenKey        = "token"
	rcloneConfigGDriveSAFileKey       = "service_account_file"
)

// from github.com/rclone/rclone/config/configfile/configfile.go:_load()
func (c *Controller) getRCloneConfig(configPath string) (err error) {
	if configPath == "" {
		err = errors.New("no config path provided")
	}
	fd, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = errors.New("config file does not exist")
		}
		return
	}
	defer fd.Close()
	// cryptReader, err := config.Decrypt(fd)
	// if err != nil {
	// 	return
	// }
	c.gc, err = goconfig.LoadFromReader(fd)
	return
}

func (c *Controller) extractDriveBackend(name string) (err error) {
	// Load backend from config if it exists
	backend, err := c.gc.GetSection(name)
	if err != nil {
		return
	}
	// Check it is a drive backend
	if backendType, found := backend[rcloneConfigTypeKey]; !found {
		return errors.New("backend type missing")
	} else if backendType != rcloneConfigGDriveTypeValue {
		return fmt.Errorf("not a drive backend: %s", backendType)
	}
	// Extract values we need
	var (
		found    bool
		tokenRaw string
	)
	if c.Drive.ClientID, found = backend[rcloneConfigGDriveClientIDKey]; !found {
		return fmt.Errorf("key %s not found", rcloneConfigGDriveClientIDKey)
	}
	if c.Drive.ClientSecret, found = backend[rcloneConfigGDriveClientSecretKey]; !found {
		return fmt.Errorf("key %s not found", rcloneConfigGDriveClientSecretKey)
	}
	if c.Drive.Scope, found = backend[rcloneConfigGDriveScopeKey]; !found {
		return fmt.Errorf("key %s not found", rcloneConfigGDriveScopeKey)
	}
	if tokenRaw, found = backend[rcloneConfigGDriveTokenKey]; found {
		if err = json.Unmarshal([]byte(tokenRaw), &c.Drive.Token); err != nil {
			return fmt.Errorf("failed to parse oauth2 token: %w", err)
		}
	} else if _, found = backend[rcloneConfigGDriveSAFileKey]; found {
		return errors.New("authentification with service account not yet implemented")
	} else {
		return errors.New("no suitable authentification found (oauth2 or service account)")
	}
	return
}
