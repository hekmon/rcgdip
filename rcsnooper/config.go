package rcsnooper

import (
	"errors"
	"fmt"
	"os"

	"github.com/Unknwon/goconfig"
	"github.com/rclone/rclone/fs/config"
)

const (
	rcloneConfigTypeKey               = "type"
	rcloneConfigGDriveTypeValue       = "drive"
	rcloneConfigGDriveClientIDKey     = "client_id"
	rcloneConfigGDriveClientSecretKey = "client_secret"
	rcloneConfigGDriveTokenKey        = "token"
	rcloneConfigGDriveSAFileKey       = "service_account_file"
)

type driveBackend struct {
	clientID     string
	clientSecret string
	token        string
}

// from github.com/rclone/rclone/config/configfile/configfile.go:_load()
func getRCloneConfig(configPath string) (gc *goconfig.ConfigFile, err error) {
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

	cryptReader, err := config.Decrypt(fd)
	if err != nil {
		return
	}

	gc, err = goconfig.LoadFromReader(cryptReader)

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
		return errors.New("type missing")
	} else if backendType != rcloneConfigGDriveTypeValue {
		return fmt.Errorf("not a GDrive backend: %s", backendType)
	}
	// Extract values we need
	var found bool
	if c.drive.clientID, found = backend[rcloneConfigGDriveClientIDKey]; !found {
		return fmt.Errorf("%s not found", rcloneConfigGDriveClientIDKey)
	}
	if c.drive.clientSecret, found = backend[rcloneConfigGDriveClientSecretKey]; !found {
		return fmt.Errorf("%s not found", rcloneConfigGDriveClientSecretKey)
	}
	if c.drive.token, found = backend[rcloneConfigGDriveTokenKey]; !found {
		if _, found = backend[rcloneConfigGDriveSAFileKey]; !found {
			return errors.New("no suitable authentification found")
		}
		return errors.New("authentification with service account not yet implemented")
	}
	return
}
