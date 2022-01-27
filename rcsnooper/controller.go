package rcsnooper

import (
	"github.com/Unknwon/goconfig"
	"golang.org/x/oauth2"
)

type Config struct {
	RCloneConfigPath string
	DriveBackendName string
}

func New(conf Config) (rcsnooper *Controller, err error) {
	rcsnooper = new(Controller)
	if rcsnooper.gc, err = getRCloneConfig(conf.RCloneConfigPath); err != nil {
		return
	}
	if err = rcsnooper.extractDriveBackend(conf.DriveBackendName); err != nil {
		return
	}
	return
}

type Controller struct {
	// rclone config
	gc    *goconfig.ConfigFile
	drive driveBackend
}

type driveBackend struct {
	clientID     string
	clientSecret string
	token        *oauth2.Token
}
