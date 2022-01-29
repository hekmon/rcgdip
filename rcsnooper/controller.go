package rcsnooper

import (
	"fmt"

	"github.com/Unknwon/goconfig"
	"golang.org/x/oauth2"
)

type Config struct {
	RCloneConfigPath string
	DriveBackendName string
}

func New(conf Config) (rcsnooper *Controller, err error) {
	rcsnooper = new(Controller)
	if err = rcsnooper.getRCloneConfig(conf.RCloneConfigPath); err != nil {
		err = fmt.Errorf("can not get RClone configuration: %w", err)
		return
	}
	if err = rcsnooper.extractDriveBackend(conf.DriveBackendName); err != nil {
		err = fmt.Errorf("can not extract drive backend '%s' from RClone configuration: %w",
			conf.DriveBackendName, err)
		return
	}
	return
}

type Controller struct {
	// rclone config
	gc    *goconfig.ConfigFile
	Drive DriveBackend
}

type DriveBackend struct {
	ClientID     string
	ClientSecret string
	Scope        string
	Token        *oauth2.Token
	RootFolderID string
	TeamDrive    string
}
