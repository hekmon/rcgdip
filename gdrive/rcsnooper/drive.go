package rcsnooper

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rclone/rclone/backend/drive"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configstruct"
	"golang.org/x/oauth2"
)

type DriveBackend struct {
	Options      drive.Options
	ClientID     string
	ClientSecret string
	Token        *oauth2.Token
}

func (c *Controller) extractDriveBackend(backend string) (err error) {
	// Load backend from config if it exists
	fsInfo, _, _, conf, err := fs.ConfigFs(backend + ":")
	if err != nil {
		return fmt.Errorf("can not get config for backend '%s': %w", backend, err)
	}
	if fsInfo.Name != "drive" {
		return errors.New("the backend needs to be of type \"drive\"")
	}
	if err = configstruct.Set(conf, &c.Drive.Options); err != nil {
		return fmt.Errorf("can not extract config of the backend '%s' as drive options: %w", backend, err)
	}
	// Extract values we need not within options
	var found bool
	if c.Drive.ClientID, found = conf.Get(config.ConfigClientID); found || c.Drive.ClientID == "" {
		return fmt.Errorf("%s must be set", config.ConfigClientID)
	}
	if c.Drive.ClientSecret, found = conf.Get(config.ConfigClientSecret); found || c.Drive.ClientSecret == "" {
		return fmt.Errorf("%s must be set", config.ConfigClientSecret)
	}
	tokenRaw, found := conf.Get(config.ConfigToken)
	if found {
		if err = json.Unmarshal([]byte(tokenRaw), &c.Drive.Token); err != nil {
			return fmt.Errorf("failed to parse oauth2 token: %w", err)
		}
	} else if c.Drive.Options.ServiceAccountFile != "" {
		return errors.New("authentification with service account not yet implemented")
	} else {
		return errors.New("no suitable authentification found (oauth2 or service account)")
	}
	return
}
